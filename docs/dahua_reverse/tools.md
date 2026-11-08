# Tools

## decrypt_xs7320_firmware.py

Standalone script that takes a Dahua XS7320 firmware ZIP and produces
all decrypted partitions plus extracted SquashFS contents.

### Requirements

```sh
pip install cryptography
# Also needs: unzip, unsquashfs (squashfs-tools)
```

### Keys

The AES keys are not stored in the repo. Provide them in one of these ways
(checked in this order):

1. `--kernel-key <hex>` / `--rootfs-key <hex>`
2. `XS7320_KERNEL_KEY` / `XS7320_ROOTFS_KEY` env vars
3. A gitignored `keys.env` file next to the script:

```sh
XS7320_KERNEL_KEY=<64 hex chars>
XS7320_ROOTFS_KEY=<64 hex chars>
```

### Usage

```sh
python3 decrypt_xs7320_firmware.py DH_SD-Taurus_MultiLang_PN_Stream3-Normal_V3.200.0000028.6.R.260514.zip -o output/
```

This will:
1. Unzip the firmware package
2. Strip uImage headers from each partition image
3. Decrypt the kernel (AES-256-CBC, skip=0x200) with the U-Boot key
4. Decrypt romfs, web, pd, and firmware (AES-256-CBC, skip=0) with the rootfs key
5. Concatenate the ext4 parts
6. Extract SquashFS images with `unsquashfs`
7. Print verification results (magic bytes, sizes, inode counts)

### Output structure

```
output/
├── fw_parts/           # Raw images extracted from the ZIP
│   ├── kernel.img
│   ├── romfs-x.squashfs.img
│   └── ...
├── decrypted/          # Decrypted partition images
│   ├── kernel.bin
│   ├── romfs.squashfs
│   ├── web.squashfs
│   ├── pd.squashfs
│   └── firmware.ext4
└── extracted/          # Extracted filesystem contents
    ├── romfs/          # Root filesystem
    ├── web/            # Web frontend
    └── pd/             # Product data
```

## Key derivation scripts (reference)

These were used during reverse engineering and are preserved for reference.
They require the Unicorn Engine (`pip install unicorn`).

### emulate_full.py

Emulates U-Boot's `block_aes_decrypt` to extract the 32-byte kernel AES key.
Requires `fw_decoded/uboot.bin` (extracted from `dhboot.bin.img`).

### emulate_key.py

Emulates the kernel's `init_key_slot` to extract the 32-byte rootfs AES key.
Requires `fw_decoded/vmlinux` (decompressed kernel).

## Verification

After decryption, verify the outputs:

```sh
# SquashFS magic
xxd -l 4 output/decrypted/romfs.squashfs    # should show: hsqs

# ext4 superblock magic
xxd -s 0x438 -l 2 output/decrypted/firmware.ext4  # should show: 53ef

# SquashFS listing
unsquashfs -l output/decrypted/romfs.squashfs | head -20

# Kernel (gzip-compressed zImage)
xxd -l 4 output/decrypted/kernel.bin | head -1  # should show gzip magic after uImage strip
```
