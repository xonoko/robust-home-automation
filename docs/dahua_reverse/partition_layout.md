# eMMC Partition Layout

The XS7320 cameras use eMMC storage with 25 partitions (`/dev/mmcblk0p1` through
`/dev/mmcblk0p25`). The partition table is passed via the kernel command line as
`blkdevparts=mmcblk0:...`.

## Partition map

Reconstructed from `mountScript.sh`, `getBootInfo`, `Install`, and the
`checkFirmware.sh` script:

| Partition | Name | Mount point | Filesystem | Access | Purpose |
|-----------|------|-------------|------------|--------|---------|
| p1 | mini-boot | — | raw | — | Minimal first-stage bootloader |
| p2 | U-Boot | — | raw | — | Main U-Boot bootloader |
| p3 | dtb | — | raw | — | Flattened device tree blob |
| p4 | (status) | — | raw | — | Upgrade status flags (byte at offset 18432) |
| p5 | kernel | — | raw | — | AES-encrypted kernel (uImage + zImage) |
| p6 | kernel-bak | — | raw | — | Backup kernel (A/B scheme) |
| p7 | hwid | — | raw | read | Hardware ID, MAC, boot params (dd-parsed) |
| p8 | mtd | `/mnt/mtd` | ext4 | RW | Config, logs, 3A calibration |
| p9 | backup | `/mnt/backup` | ext4 | RW | Config backup, debug data |
| p10 | syslog | `/mnt/syslog` | ext4 | RW | System/web optimization logs |
| p11 | devconfig | `/mnt/devconfig` | ext4 | RW | Device config, firmware_corrupt flag |
| p12 | pd | `/mnt/pd` | SquashFS | RO | Product data (HWID table, defaults, SDCameras) |
| p13 | pd-bak | — | SquashFS | — | Backup pd |
| p14 | web | `/mnt/web` | SquashFS | RO | Web frontend (React SPA + WASM decoders) |
| p15 | web-bak | — | SquashFS | — | Backup web |
| p16–p19 | rootfs, rootfs-bak, ... | `/` | SquashFS | RO | Root filesystem (2× for A/B) |
| p20–p21 | (reserved) | — | — | — | |
| p22 | firmware | `/mnt/firmware` | ext4 | RO | Main application data (AI models, plugins) |
| p23 | sfmw | `/mnt/sfmw` | SquashFS | RO | Supplementary firmware |
| p24 | Ddump | `/mnt/Ddump` | ext4 | RW | Core dumps (coredump target) |
| p25 | appdata | `/mnt/appdata` | ext4 | RW | Application runtime data |

## Mount sequence

`mountScript.sh` accepts a mode argument (`R`, `W`, or `RW`) and mounts
partitions accordingly:

- **RW partitions** (p8, p9, p10, p11, p24, p25): `e2fsck -p` first, then
  `mount -t ext4`. If mount fails, the partition is reformatted with `mke2fs`
  and remounted. This self-healing means the camera recovers from ext4
  corruption automatically.

- **RO partitions** (p12, p14, p22, p23): mounted directly, some with `-o ro`.

The `rcS` init script calls `mountScript.sh RW` early in boot, mounting both
R and W partitions.

## hwid partition

Partition p7 stores hardware identity. It's read by `getBootInfo` using
raw `dd`:

```sh
dd if=/dev/mmcblk0p${hwid_partition} bs=4 skip=1 count=32768
```

The extracted data is grep-parsed for these fields:
- `HWID` — product model string (e.g. `CA-HZ4032GB-C`)
- `appauto` — 0/1, controls whether app.sh runs
- `ethaddr` / `wifiaddr` — MAC addresses
- `dh_keyboard` — 0/1, controls serial console access
- `ID` — device ID
- `SWID` — software ID
- `sysbackup` — 0/1, backup mode flag

The `HWID` determines which product profile is loaded from `product.cfg`.

## Firmware integrity

`checkFirmware.sh` reads two bytes from the upgrade status partition (p4):
- Byte at offset **18432**: 'E' = upgrade failed, 'J' = upgrade success
- Byte at offset **0**: 'U' = upgrade in progress

On failure, it creates `/mnt/devconfig/firmware_corrupt.flag` and unmounts
the firmware and sfmw partitions — the camera falls back to backup partitions.
