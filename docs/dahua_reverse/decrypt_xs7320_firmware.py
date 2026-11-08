#!/usr/bin/env python3
"""
Dahua XS7320 (Taurus) firmware decryptor.

Decrypts all encrypted partitions from a Dahua firmware ZIP package targeting
the CHIPUP XS7320 SoC family (SD-Taurus PTZ cameras).

Two AES-256-CBC keys, both derived from a shared sha_init_data blob baked into
U-Boot and the kernel. The first 16 bytes of each key are the "fw_key"
(SHA-256 of modified sha_init_data, XOR-folded); the second 16 bytes come from
SoC-specific key derivation tables (different table entries for each key).

Keys were extracted via Unicorn emulation of the actual ARM binaries — see
emulate_full.py and emulate_key.py for the extraction scripts.

Usage:
    python3 decrypt_xs7320_firmware.py <firmware.zip> [-o output_dir]

Requirements:
    pip install cryptography
    External: unzip, unsquashfs (squashfs-tools package)
"""
import argparse
import os
import shutil
import struct
import subprocess
import sys
import tempfile
import zipfile

from cryptography.hazmat.primitives.ciphers import Cipher, algorithms, modes

# AES-256 keys for the XS7320 SoC family (per-SoC-family, not per-device).
# Not stored in the repo: pass them via --kernel-key/--rootfs-key, the
# XS7320_KERNEL_KEY/XS7320_ROOTFS_KEY env vars, or a keys.env file next to
# this script. Each key is 64 hex chars: fw_key (16 bytes) + derivation (16).
KEYS_FILE = os.path.join(os.path.dirname(os.path.abspath(__file__)), "keys.env")


def load_key(name: str, cli_value: str | None) -> bytes:
    """Resolve a key from CLI arg, then env var, then keys.env."""
    value = cli_value or os.environ.get(name)
    if not value and os.path.isfile(KEYS_FILE):
        with open(KEYS_FILE) as f:
            for line in f:
                k, _, v = line.strip().partition("=")
                if k == name:
                    value = v.strip()
    if not value:
        sys.exit(f"Error: missing {name} (use CLI flag, env var, or keys.env)")
    key = bytes.fromhex(value)
    if len(key) != 32:
        sys.exit(f"Error: {name} must be 32 bytes (64 hex chars)")
    return key

AES_BLOCK = 0x200  # 512-byte cipher blocks

UIMAGE_MAGIC = 0x27051956
SQUASHFS_MAGIC = b"hsqs"


def get_iv(block_num: int) -> bytes:
    """Per-block IV: little-endian block counter + 12 zero bytes."""
    return struct.pack("<I", block_num) + b"\x00" * 12


def strip_uimage(data: bytes) -> tuple[bytes, int, str]:
    """Strip one or two uImage headers. Returns (payload, comp_type, name)."""
    if len(data) < 64 or struct.unpack(">I", data[:4])[0] != UIMAGE_MAGIC:
        return data, -1, ""
    comp = data[31]
    # Check for double uImage header
    if len(data) > 128 and struct.unpack(">I", data[64:68])[0] == UIMAGE_MAGIC:
        comp2 = data[64 + 31]
        name = data[96:128].split(b"\x00")[0].decode("ascii", "replace")
        return data[128:], comp2, name
    name = data[32:64].split(b"\x00")[0].decode("ascii", "replace")
    return data[64:], comp, name


def decrypt_aes256_cbc(payload: bytes, key: bytes, skip_bytes: int = 0) -> bytes:
    """
    Decrypt AES-256-CBC with per-512-byte-block counter IV.

    skip_bytes: number of plaintext bytes between each encrypted block.
      0 = fully encrypted (rootfs scheme).
      0x200 = alternating encrypted/plain 512-byte blocks (kernel scheme).
    """
    result = bytearray()
    block_num = 0
    pos = 0
    total = len(payload)

    while pos < total:
        enc = payload[pos : pos + AES_BLOCK]
        if not enc:
            break
        if len(enc) < 16:
            result.extend(enc)
            break

        usable = len(enc) - (len(enc) % 16) if len(enc) % 16 else len(enc)
        iv = get_iv(block_num)
        cipher = Cipher(algorithms.AES(key), modes.CBC(iv))
        dec = cipher.decryptor()
        decrypted = dec.update(enc[:usable]) + dec.finalize()
        result.extend(decrypted[: len(enc)])
        pos += AES_BLOCK

        if skip_bytes > 0:
            plain = payload[pos : pos + skip_bytes]
            result.extend(plain)
            pos += skip_bytes

        block_num += 1

    return bytes(result)


def identify(data: bytes) -> str:
    """Identify a decrypted image by magic bytes."""
    if data[:4] == SQUASHFS_MAGIC:
        return "squashfs"
    if data[:2] == b"\x1f\x8b":
        return "gzip"
    if len(data) > 0x43A and data[0x438:0x43A] == b"\x53\xef":
        return "ext4"
    if len(data) >= 4 and struct.unpack(">I", data[:4])[0] == UIMAGE_MAGIC:
        return "uimage"
    return "unknown"


def process_image(
    src_path: str, key: bytes, skip: int, out_path: str, label: str
) -> bool:
    """Decrypt a single partition image. Returns True on success."""
    if not os.path.exists(src_path):
        print(f"  SKIP {label}: {os.path.basename(src_path)} not found")
        return False

    raw = open(src_path, "rb").read()
    payload, comp, name = strip_uimage(raw)
    dec = decrypt_aes256_cbc(payload, key, skip)
    fmt = identify(dec)

    os.makedirs(os.path.dirname(out_path), exist_ok=True)
    with open(out_path, "wb") as f:
        f.write(dec)

    status = "OK" if fmt != "unknown" else "UNKNOWN FORMAT"
    print(f"  {label}: {len(raw)} → {len(dec)} bytes, detected {fmt} [{status}]")
    return fmt != "unknown"


def extract_squashfs(squashfs_path: str, dest_dir: str, label: str) -> bool:
    """Extract a SquashFS image using unsquashfs."""
    if not shutil.which("unsquashfs"):
        print(f"  {label}: unsquashfs not found, skipping extraction")
        return False

    if os.path.exists(dest_dir):
        shutil.rmtree(dest_dir)

    try:
        result = subprocess.run(
            ["unsquashfs", "-d", dest_dir, "-no-progress", "-f", squashfs_path],
            capture_output=True,
            text=True,
            timeout=120,
        )
        if result.returncode == 0:
            count = sum(1 for _, _, files in os.walk(dest_dir) for _ in files)
            print(f"  {label}: extracted to {dest_dir} ({count} files)")
            return True
        else:
            print(f"  {label}: unsquashfs failed: {result.stderr[:200]}")
            return False
    except subprocess.TimeoutExpired:
        print(f"  {label}: unsquashfs timed out")
        return False


def unpack_zip(zip_path: str, dest_dir: str) -> str:
    """
    Unpack the firmware package.

    Dahua firmware is distributed as:
      outer.zip → contains a DHSP .bin file (+ README.txt)
      DHSP .bin → a ZIP with a small "DHSP" header prepended, containing
                  the actual partition .img files, Install, hwid, etc.

    We use the system `unzip` command which handles prepended data gracefully.
    """
    os.makedirs(dest_dir, exist_ok=True)

    # Extract outer ZIP
    subprocess.run(
        ["unzip", "-o", "-q", zip_path, "-d", dest_dir],
        check=True,
    )

    # Look for a .bin DHSP container inside and extract it too.
    # The DHSP header overwrites the first ZIP entry's local header,
    # so unzip warns (exit code 1) but still extracts all other entries.
    for root, _, files in os.walk(dest_dir):
        for f in files:
            if f.endswith(".bin"):
                bin_path = os.path.join(root, f)
                with open(bin_path, "rb") as bf:
                    magic = bf.read(4)
                if magic == b"DHSP":
                    result = subprocess.run(
                        ["unzip", "-o", "-q", bin_path, "-d", dest_dir],
                        capture_output=True,
                    )
                    if result.returncode > 1:
                        print(f"  Warning: unzip exited {result.returncode}")
                        print(f"  {result.stderr.decode(errors='replace')[:200]}")
                    os.unlink(bin_path)
                    break

    return dest_dir


def find_parts(parts_dir: str) -> dict[str, str]:
    """Locate the partition image files in the extracted directory."""
    names = {}
    for root, _, files in os.walk(parts_dir):
        for f in files:
            if f.endswith(".img") or f in ("Install", "hwid"):
                names[f] = os.path.join(root, f)
    return names


def main():
    parser = argparse.ArgumentParser(
        description="Decrypt Dahua XS7320 (Taurus) firmware"
    )
    parser.add_argument("firmware", help="Firmware ZIP file path")
    parser.add_argument(
        "-o",
        "--output",
        default="dahua_decoded",
        help="Output directory (default: dahua_decoded)",
    )
    parser.add_argument(
        "--no-extract",
        action="store_true",
        help="Skip SquashFS extraction (decrypt only)",
    )
    parser.add_argument("--kernel-key", help="Kernel AES-256 key (hex)")
    parser.add_argument("--rootfs-key", help="Rootfs AES-256 key (hex)")
    args = parser.parse_args()

    KERNEL_KEY = load_key("XS7320_KERNEL_KEY", args.kernel_key)
    ROOTFS_KEY = load_key("XS7320_ROOTFS_KEY", args.rootfs_key)

    if not os.path.isfile(args.firmware):
        print(f"Error: {args.firmware} not found")
        sys.exit(1)

    out = os.path.abspath(args.output)
    parts_dir = os.path.join(out, "fw_parts")
    dec_dir = os.path.join(out, "decrypted")
    ext_dir = os.path.join(out, "extracted")

    # Step 1: Unpack
    print("=" * 60)
    print("Dahua XS7320 Firmware Decryptor")
    print("=" * 60)
    print(f"\nInput:  {args.firmware}")
    print(f"Output: {out}")

    print("\n[1/4] Unpacking firmware ZIP...")
    unpack_zip(args.firmware, parts_dir)
    parts = find_parts(parts_dir)
    print(f"  Found {len(parts)} files: {', '.join(sorted(parts.keys()))}")

    # Step 2: Decrypt
    print("\n[2/4] Decrypting partition images...")
    os.makedirs(dec_dir, exist_ok=True)

    ok = True
    ok &= process_image(
        parts.get("kernel.img", ""),
        KERNEL_KEY,
        0x200,
        os.path.join(dec_dir, "kernel.bin"),
        "kernel",
    )
    for src_name, out_name, label in [
        ("romfs-x.squashfs.img", "romfs.squashfs", "romfs"),
        ("web-x.squashfs.img", "web.squashfs", "web"),
        ("pd-x.squashfs.img", "pd.squashfs", "pd"),
    ]:
        ok &= process_image(
            parts.get(src_name, ""),
            ROOTFS_KEY,
            0,
            os.path.join(dec_dir, out_name),
            label,
        )

    # Decrypt and concatenate ext4 parts
    ext4_parts = []
    for suffix in ("_aa", "_ab", "_ac"):
        src_name = f"firmware-x.ext4{suffix}.img"
        out_name = f"firmware{suffix}.ext4"
        src = parts.get(src_name, "")
        dst = os.path.join(dec_dir, out_name)
        if os.path.exists(src):
            process_image(src, ROOTFS_KEY, 0, dst, f"firmware{suffix}")
            ext4_parts.append(dst)

    if ext4_parts:
        combined = os.path.join(dec_dir, "firmware.ext4")
        print("  Concatenating ext4 parts...")
        with open(combined, "wb") as out_f:
            for p in ext4_parts:
                with open(p, "rb") as pf:
                    shutil.copyfileobj(pf, out_f)
        size_mb = os.path.getsize(combined) / 1e6
        fmt = identify(open(combined, "rb").read(0x440))
        print(f"  firmware.ext4: {size_mb:.1f} MB, detected {fmt}")

    # Step 3: Verify
    print("\n[3/4] Verifying decrypted images...")
    errors = []
    for name, expected in [
        ("romfs.squashfs", "squashfs"),
        ("web.squashfs", "squashfs"),
        ("pd.squashfs", "squashfs"),
        ("firmware.ext4", "ext4"),
    ]:
        path = os.path.join(dec_dir, name)
        if not os.path.exists(path):
            errors.append(f"  MISSING: {name}")
            continue
        with open(path, "rb") as f:
            head = f.read(0x440)
        fmt = identify(head)
        if fmt == expected:
            print(f"  OK: {name} ({fmt})")
        else:
            errors.append(f"  FAIL: {name} — expected {expected}, got {fmt}")

    # kernel: should be gzip (zImage) or uimage
    kpath = os.path.join(dec_dir, "kernel.bin")
    if os.path.exists(kpath):
        with open(kpath, "rb") as f:
            khead = f.read(128)
        kfmt = identify(khead)
        if kfmt in ("gzip", "uimage"):
            print(f"  OK: kernel.bin ({kfmt})")
        else:
            errors.append(f"  WARN: kernel.bin — got {kfmt} (may still be valid zImage)")

    if errors:
        print("\nVerification issues:")
        for e in errors:
            print(e)
    else:
        print("  All images verified successfully.")

    # Step 4: Extract SquashFS
    if not args.no_extract:
        print("\n[4/4] Extracting SquashFS images...")
        os.makedirs(ext_dir, exist_ok=True)
        for name, label in [
            ("romfs.squashfs", "romfs"),
            ("web.squashfs", "web"),
            ("pd.squashfs", "pd"),
        ]:
            sqfs = os.path.join(dec_dir, name)
            if os.path.exists(sqfs):
                extract_squashfs(sqfs, os.path.join(ext_dir, label), label)
    else:
        print("\n[4/4] Skipping SquashFS extraction (--no-extract)")

    print("\nDone.")
    print(f"Decrypted images: {dec_dir}/")
    if not args.no_extract:
        print(f"Extracted filesystems: {ext_dir}/")


if __name__ == "__main__":
    main()
