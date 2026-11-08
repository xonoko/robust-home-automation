# Boot Sequence

## Overview

```
Power on
  → mini-boot (p1) — minimal hardware init
  → U-Boot (p2) — loads DTB (p3), decrypts kernel (p5), boots Linux
  → Linux 4.19.132 — mounts rootfs (SquashFS via block-layer decryption)
  → /linuxrc (busybox init) — basic init
  → /etc/init.d/rcS — mounts partitions, loads drivers, sets up config
  → /etc/init.d/appauto — if appauto=1, runs /usr/etc/app.sh
  → /usr/etc/user.sh — kernel tunables (VM, network, coredump)
  → /usr/etc/app.sh — watchdogs, then launches sonia (the main application)
```

## U-Boot stage

U-Boot (`dhboot.bin.img`, 200 KB) is an unencrypted uImage. It:

1. Initializes the XS7320 hardware (DDR, clocks, eMMC)
2. Reads the DTB from partition p3
3. Reads the encrypted kernel from partition p5
4. Decrypts it using `block_aes_decrypt` with the kernel key and skip=0x200
5. Boots the kernel with the DTB and a command line containing `blkdevparts=`

## Kernel boot

The kernel (Linux 4.19.132 for ARM) has a custom block-layer hook that
transparently decrypts reads from SquashFS and ext4 partitions. This means
the rootfs SquashFS is stored encrypted on eMMC and decrypted on-the-fly
as blocks are read — there's no "decrypt to RAM" step.

The decryption is in a function at `0xC05C98A8` that calls `block_aes_decrypt`
with type=0x11 (key index 1, decrypt mode) and skip=0 (fully encrypted).

## /etc/init.d/rcS

The main init script:

1. Sources `getBootInfo` — reads HWID, MAC, appauto, keyboard flags from
   the hwid partition via `dd`
2. Runs `mountScript.sh RW` — mounts all ext4 and SquashFS partitions
3. Runs `checkFirmware.sh` — validates upgrade status
4. Loads kernel modules:
   - `osa.ko` — OS abstraction layer
   - `logV2.ko` — Dahua logging subsystem
   - `dgs.ko` — device group subsystem
   - `binder.ko` — Android-style IPC (r0size=16, r2size=320)
   - SDK modules: `xs7320_nial.ko`, `gpio.ko`, `i2c.ko`, `adc.ko`,
     `pwm.ko`, `watchdog.ko`, `spi.ko`, `rtc.ko`
   - `prc.ko` — process control
5. Creates directories under `/mnt/mtd/Config`, `/mnt/backup`, `/var/tmp`
6. Parses `product.cfg` to load the correct hardware profile based on HWID

## /usr/etc/app.sh

The application launcher:

1. Sets up coredump handling: `core_pattern = |/usr/bin/core_zlib.bin /mnt/Ddump %e-%p-%s`
2. Feeds the hardware watchdog (`/proc/osa_root/pdc/pdcWdt`)
3. Tunes TCP write buffers: `tcp_wmem = 4096 1572864 2072640`
4. Starts the system watchdog monitor (`ipdtest_wdt.sh system`)
5. Starts the sonia watchdog monitor (`ipdtest_wdt.sh sonia &`)
6. Launches **sonia** (or `sonia_agent` for non-zero chip grades)

## /usr/etc/user.sh

Kernel tunables set before sonia starts:

```sh
dirty_writeback_centisecs = 100    # flush cache every 1s
dirty_expire_centisecs = 500       # cache expires after 5s
vfs_cache_pressure = 50            # favor keeping dentry/inode caches
swappiness = 30                    # prefer keeping pages in RAM
dirty_ratio = 10                   # synchronous writeback at 10%
dirty_bytes = 1048576              # 1 MB dirty threshold
dirty_background_bytes = 1048576   # 1 MB background writeback
wmem_max = 2097152                 # 2 MB socket write buffer
wmem_default = 2097152             # 2 MB default socket buffer
overcommit_memory = 1              # always overcommit
overcommit_ratio = 90              # allow 90% overcommit
```

## Serial console

If `dh_keyboard=0` (normal operation), `/dev/ttyS0` is linked to `/dev/mytty`
and used for serial output. If `dh_keyboard=1`, a null device is used instead
(serial disabled — kiosk/standalone mode).

SSH runs OpenSSH (`/usr/local/sbin/sshd`) with:
- `PermitRootLogin yes`
- `PermitEmptyPasswords yes`
- `PubkeyAuthentication no`
- `AllowUsers admin`
- Only modern ciphers (aes-ctr, aes-gcm) and KEX (curve25519, DH group16/18)
