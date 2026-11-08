# Filesystem Contents

## romfs (rootfs) — 71.9 MB, 2876 inodes, SquashFS v4 XZ

The root filesystem. Mounted as `/` (read-only).

### Key binaries

| Path | Size | Purpose |
|------|------|---------|
| `/usr/bin/sonia` | 67 MB | Main application: video pipeline, AI, RPC, web server, PTZ, ONVIF, everything |
| `/usr/bin/CManager` | 67 KB | Configuration manager |
| `/usr/bin/logd` | — | Log daemon |
| `/usr/bin/xwatchdog` | — | Watchdog feeder |
| `/usr/bin/xbroadcast` | — | Network broadcast (discovery) |
| `/usr/bin/xftp` | — | FTP server |
| `/usr/bin/RPCserver` | — | RPC2 endpoint |
| `/usr/bin/secboot` | — | Secure boot verification |
| `/usr/bin/core_zlib.bin` | — | Compressed coredump handler |
| `/usr/bin/lua` | — | Lua interpreter (scripting engine) |
| `/usr/bin/snmpd` | — | SNMP daemon |
| `/bin/busybox` | 486 KB | Standard UNIX utilities (ash, awk, dd, grep, mount, etc.) |

### User accounts

From `/etc/passwd`:
```
root:x:0:0:root:/:/bin/sh
sshd:x:74:74:Privilege-separated SSH:/var/empty/sshd:/sbin/nologin
admin:x:0:502:Linux User,,,:/:/bin/dsh
```

The `admin` user has UID 0 (root equivalent) and shell `/bin/dsh` (Dahua
restricted shell). No `/etc/shadow` file — password hashing is handled by
the application.

### Libraries

**System (glibc 2.28)**:
```
/lib/ld-2.28.so, libc-2.28.so, libm-2.28.so, libpthread-2.28.so,
librt-2.28.so, libdl-2.28.so, libstdc++.so.6.0.25, libcrypt-2.28.so
```

**Dahua proprietary** (`/usr/lib/`):
| Library | Purpose |
|---------|---------|
| `libdhivs_sdk_ivs_debug.so` | Intelligent Video Surveillance (IVS) |
| `libdhivs_sdk_face_debug.so` | Face detection |
| `libdhivs_sdk_track_debug.so` | Object tracking |
| `libdhivs_sdk_smokefire.so` | Smoke/fire detection |
| `libdhivs_sdk_bmd_debug.so` | Background model detection |
| `libai_sdk_feature_extract_debug.so` | AI feature extraction |
| `libai_sdk_stereopc.so` | Stereo point cloud |
| `libdh_npu_lib.so` | NPU (neural processing unit) interface |
| `libFRI_SEC_SDK.so` | Security/encryption SDK |
| `libGb35114Mgr.so` | GB/T 35114 (Chinese video security standard) |
| `libmdk.so` | Media development kit |
| `libVIPlite.so` / `libVIPuser.so` | VIP (video/image processing) hardware abstraction |

### Kernel modules

**SDK modules** (`/usr/lib/modules/sdk/`):
| Module | Purpose |
|--------|---------|
| `xs7320_base.ko` | SoC base driver |
| `xs7320_nial.ko` | Network interface abstraction |
| `xs7320_vi.ko` | Video input |
| `xs7320_vo.ko` | Video output |
| `xs7320_venc.ko` | Video encoder (H.264/H.265) |
| `xs7320_isp.ko` | Image signal processor |
| `xs7320_ive.ko` | Intelligent video engine |
| `xs7320_vps.ko` | Video processing subsystem |
| `xs7320_vefw_host.ko` (1 MB) | Video encoder firmware (host side) |
| `xs7320_vefw_coproc.ko` (551 KB) | Video encoder firmware (coprocessor) |
| `xs7320_cipher.ko` | Hardware crypto engine |
| `xs7320_audio.ko` | Audio subsystem |
| `xs7320_dvp.ko` | Digital video port |
| `xs7320_osd.ko` | On-screen display |
| `xs7320_ldc.ko` | Lens distortion correction |
| `xs7320_rotate.ko` | Image rotation |
| `xs7320_clock.ko` | Clock management |
| `xs7320_codec.ko` | Audio codec |
| `xs7320_athena.ko` | AI/NPU accelerator interface |
| `vipcore.ko` | VIP core hardware driver |
| `mmz.ko` | Media memory zone (CMA allocator) |

**Peripheral modules** (`/usr/lib/modules/`):
| Module | Purpose |
|--------|---------|
| `sensor.ko` | Image sensor driver |
| `motorDrv.ko` | PTZ motor driver |
| `mult_len_driver.ko` | Multi-lens controller |
| `8192eu.ko`, `8811au.ko`, `aic8800_netdrv.ko` | WiFi drivers |
| `GobiNet5G.ko`, `cdc_ether.ko`, `usbnet.ko` | Cellular/USB networking |
| `lt9211c.ko` | LVDS/MIPI bridge |

## web — 19.9 MB, 1343 files, SquashFS v4 XZ

The web frontend. Mounted at `/mnt/web`.

A React single-page application with:
- Webpack-bundled JS/CSS in `static/js/` and `static/css/`
- WASM decoders: `libDecodeSDK.wasm`, `MPA_DHIVS_AutoMatch.wasm`, `MPA_HSPano.wasm`
- 8 Web Workers for parallel video decoding
- Plugin system: `webplugin.exe` (ActiveX for IE), `plugin.data`
- Multi-language support: 24 languages in `lang/`
- Module system: `module/` directory with per-feature JS bundles
- `H5Player` — HTML5 video player using WASM for codec support

## pd (product data) — 5.3 MB, 375 inodes, SquashFS v4 XZ

Mounted at `/mnt/pd`. Contains:

- `SDCameras/` — hardware profiles for ~30 SD/PTZ camera models
  (lens parameters, motor curves, zoom tables)
- `Export.cfg` — export configuration
- `FactoryDefault.WhiteList` / `FactoryDefault.ResetDir` — factory reset scope
- `FuzzyConfig.json`, `MaskConfig.json` — image processing defaults
- `StringsOpenSourceNotice`, `StringsPrivacyPolicy`, `StringsSoftwareLicense` — legal
- `AbbreviationStr/` — localized string abbreviations
- `custom/` — customization overlays

## firmware.ext4 — 337.6 MB, 20610 inodes

Mounted at `/mnt/firmware` (read-only). This is the largest partition and
contains the bulk of the application data:

- AI/deep learning models for IVS features
- Additional language packs
- Supplementary plugins and libraries
- Large media assets

The ext4 image is split into 3 parts in the firmware package due to the
`spilteSize` limit of 128 MB. They must be concatenated before mounting:

```sh
cat firmware_aa.ext4 firmware_ab.ext4 firmware_ac.ext4 > firmware.ext4
```
