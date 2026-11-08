# Dahua XS7320 Firmware Reverse Engineering

Complete documentation for decrypting, extracting, and understanding the firmware
of Dahua PTZ cameras based on the CHIPUP XS7320 SoC (dual-core ARM Cortex-A7).

Target device: **DH-SD5A425GB-HNR** (firmware V3.200.0000028.6.R.260514).

## Documents

| File | Contents |
|------|----------|
| [firmware_structure.md](firmware_structure.md) | ZIP/uImage packaging, partition images, Install manifest |
| [encryption.md](encryption.md) | AES-256-CBC scheme for kernel and rootfs, key derivation, Unicorn emulation |
| [partition_layout.md](partition_layout.md) | eMMC partition map, mount points, filesystem types |
| [boot_sequence.md](boot_sequence.md) | U-Boot → kernel → rcS → sonia startup chain |
| [rootfs_anatomy.md](rootfs_anatomy.md) | Romfs, web, pd, firmware.ext4 — what's in each filesystem |
| [soc_and_hardware.md](soc_and_hardware.md) | XS7320 SoC, kernel modules, SDK components |
| [tools.md](tools.md) | decrypt_fw.py, emulate_full.py — how to use the scripts |

## Quick reference

```
SoC:           CHIPUP XS7320 (dual ARM Cortex-A7 @ 1 GHz)
Kernel:        Linux 4.19.132
Userland:      glibc 2.28, busybox, OpenSSH
Main binary:   /usr/bin/sonia (67 MB, stripped ELF)
Web frontend:  React SPA with WASM decoders (H5Player, DHIVS)
Storage:       eMMC, 25 partitions (mmcblk0p1–p25)
Encryption:    AES-256-CBC, per-SoC-family keys derived from sha_init_data
```

## Prior art

- Yuri Vasin, "Dahua firmware encryption", OFFZONE 2022 — describes the
  DHSP header and the U-Boot `block_aes_decrypt` function. Our work extends
  this to the *kernel-side* rootfs decryption, which uses a different key
  derivation path and different encryption parameters.
