# Firmware Encryption

The XS7320 uses **two separate AES-256-CBC encryption schemes** — one for the
kernel (decrypted by U-Boot) and one for every other filesystem image
(decrypted by the kernel's block layer at read time).

Both share the same first 16 bytes of their 32-byte key (`fw_key`), but the
second 16 bytes are derived differently.

## Common: fw_key derivation

Both U-Boot and the kernel contain an identical `firmware_get_key` function
and an identical 256-byte `sha_init_data` blob.

```
Algorithm:
  1. Copy sha_init_data[256] into a working buffer
  2. Overwrite the first 6 bytes with the SoC identifier "xs7320"
  3. SHA-256 hash the 256-byte buffer → 32-byte digest
  4. XOR the first 16 bytes with the last 16 bytes → 16-byte fw_key
```

This produces the same `fw_key` on both paths:

```
fw_key = <redacted — 16 bytes>
```

The `sha_init_data` blob and the SoC string are the only inputs. Every camera
on the same SoC family (XS7320) shares the same fw_key. This is not
per-device — it's per-SoC-family.

### Addresses

| Symbol | U-Boot offset | Kernel vaddr |
|--------|--------------|--------------|
| `sha_init_data` | 0x120277b4 | 0xC0800178 |
| SoC string "xs7320" | (inline) | 0xC08A7640 |
| `firmware_get_key` | 0x1e73c | 0xC0202CA4 |

## Scheme 1: Kernel decryption (U-Boot)

U-Boot's `block_aes_decrypt` at offset `0x1e898`:

1. Calls `firmware_get_key` → writes 16-byte fw_key to a stack buffer
2. Calls `aes_setkey_full(key_buf, 0x10, 0)` — this function:
   - Uses slot 0x10 (16) as an index into a key derivation table
   - Runs `set_slot_val_by_default` to derive 32 bytes
   - Overwrites the **first 16 bytes** of the 32-byte result with fw_key
   - Stores the final 32-byte AES key in a global context
3. Calls `aes_process` with skip=0x200

**Key**: `<fw_key>` + `<slot 0x10 derivation>` (32 bytes, redacted — see `keys.env`)

**Encryption parameters**:
- AES-256-CBC
- Block size: 0x200 (512 bytes)
- IV per block: `LE32(block_counter) || 00 00 00 00 00 00 00 00 00 00 00 00`
- **Skip**: 0x200 — alternating 0x200 encrypted, 0x200 plaintext
- The skip means only half the kernel payload is actually encrypted

## Scheme 2: Rootfs decryption (kernel block layer)

The kernel's `block_aes_decrypt` at vaddr `0xC020281C` is called from a single
site at `0xC05C9B44`, inside the rootfs block-read function (starts at
`0xC05C98A8`, identified by the error string "rootfs encrypt file" at
`0xC08A7574`).

The call passes `R3 = 0x11`, which encodes:

```
Lower nibble = 1  → key index (into a 3-entry table at 0xC0B38600)
Upper nibble = 1  → mode (1 = decrypt)
```

The key setup path:

1. `init_key_slot(1)` is called during kernel init:
   - Calls `derive_key(1)` → produces a full 32-byte derived key
   - Stores it in key_table[1] (at 0xC0B38620)
2. A separate wrapper at `0xC0202D68` calls `firmware_get_key` → fw_key,
   then `set_key(fw_key, 16, 1)` — which overwrites **only the first 16
   bytes** of key_table[1] with fw_key
3. The final 32-byte key at table[1] is: `[fw_key (16)] + [derive_key output (16)]`

**Key**: `<fw_key>` + `<index-1 derivation>` (32 bytes, redacted — see `keys.env`)

**Encryption parameters**:
- AES-256-CBC
- Block size: 0x200 (512 bytes)
- IV per block: `LE32(block_counter) || 00 00 00 00 00 00 00 00 00 00 00 00`
- **Skip**: 0 — the entire payload is encrypted (no alternating plaintext)

This key decrypts: romfs, web, pd (all SquashFS), and firmware (ext4, split
into 3 parts that must be concatenated).

## Type parameter encoding

The `type` parameter passed to `aes_process` is a single byte encoding both
the key index and the operation mode:

```
bits [3:0] = key_index  (0, 1, or 2 — indexes a 3-entry table of 32-byte keys)
bits [7:4] = mode        (0 = encrypt, 1 = decrypt)
```

| Value | Index | Mode | Used by |
|-------|-------|------|---------|
| 0x10 | 0 | decrypt | U-Boot kernel decryption |
| 0x11 | 1 | decrypt | Kernel rootfs/web/pd/firmware decryption |
| 0x00 | 0 | encrypt | U-Boot kernel encryption (build-time) |
| 0x01 | 1 | encrypt | Kernel filesystem encryption (build-time) |

Index 2 exists in the table but no caller was found using it.

## Key table

The kernel maintains a global key table at BSS address `0xC0B38600`:

```
Table[0]: 32 bytes at 0xC0B38600  (kernel key — same as U-Boot slot 0x10)
Table[1]: 32 bytes at 0xC0B38620  (rootfs key)
Table[2]: 32 bytes at 0xC0B38640  (unused/reserved)
```

There's also a global crypto context structure at `0xC0B385F0` that holds
the expanded AES key schedule after `set_key` is called.

## Key extraction via Unicorn emulation

Since the key derivation involves proprietary algorithms (not just standard
crypto primitives), we extracted the keys by emulating the actual ARM code
with the Unicorn Engine rather than reimplementing the algorithm in Python.

### U-Boot key (emulate_full.py)

Emulates `block_aes_decrypt` starting at offset `0x1e898`, hooks at the
`aes_process` BL instruction (`0x1e90c`) to stop execution after key setup,
then searches mapped memory for the fw_key pattern to find the full 32-byte key.

### Kernel rootfs key (emulate_key.py)

Emulates the kernel's `init_key_slot(1)` → `derive_key(1)` → `set_key`
sequence, using the decompressed vmlinux binary loaded at its virtual address
base. Hooks are installed to skip hardware-dependent operations (printf, etc.).

The kernel binary (`vmlinux`, 11.7 MB) was obtained by:
1. Decrypting `kernel.img` with the U-Boot key (skip=0x200)
2. Stripping the uImage header → zImage
3. Extracting the gzip-compressed payload from the zImage self-extractor

### Key verification

Both keys were verified by decrypting their respective images and checking
for valid filesystem magic bytes:
- SquashFS: `hsqs` at offset 0
- ext4: `0x53EF` at offset 0x438
- zImage: `0x1F8B` gzip header

## Why not just SHA-256 + XOR?

An early attempt to reimplement `firmware_get_key` in pure Python produced a
*different* fw_key than what the actual binary computes. The likely cause is a
subtle difference in the `sha_init_data` offset or the XOR fold logic.

The Unicorn approach is strictly more reliable: it runs the exact same ARM
instructions as the real hardware, byte for byte. The pure-Python approach
should be considered unreliable unless validated against emulation output.
