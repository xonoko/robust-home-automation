# SoC and Hardware

## CHIPUP XS7320

| Property | Value |
|----------|-------|
| Vendor | CHIPUP (大华旗下) — Dahua's in-house silicon division |
| CPU | Dual-core ARM Cortex-A7 @ 1 GHz |
| Memory | DDR3/DDR4 (size varies by model) |
| Storage | eMMC (25 partitions) |
| Video | ISP + hardware H.264/H.265 encoder (coprocessor) |
| AI | Athena NPU for IVS (face, vehicle, fire, tracking) |
| Crypto | Hardware AES engine (`xs7320_cipher.ko`) |
| Video I/O | MIPI CSI, DVP, LVDS (via lt9211c bridge) |
| Audio | I2S codec, ADC |
| GPIO | Multi-purpose (PTZ motor, IR cut, alarm I/O) |

The XS7320 is Dahua's "Taurus" platform — the firmware family identifier
`SD-Taurus` in the filename refers to this SoC generation.

## Video pipeline

```
Sensor → ISP (xs7320_isp.ko) → VI (xs7320_vi.ko) → VPS (xs7320_vps.ko)
  → VENC (xs7320_venc.ko + vefw_host/coproc) → RTSP/HTTP streams
  → VO (xs7320_vo.ko) → HDMI/CVBS output
  → IVE (xs7320_ive.ko) → AI analytics
  → OSD (xs7320_osd.ko) → timestamp/text overlays
  → LDC (xs7320_ldc.ko) → lens distortion correction
```

The video encoder runs as a coprocessor — the `xs7320_vefw_host.ko` (1 MB)
and `xs7320_vefw_coproc.ko` (551 KB) modules load firmware onto a dedicated
core that handles H.264/H.265 encoding independently of the A7 CPUs.

## Memory architecture

The `mmz.ko` (Media Memory Zone) module implements a CMA-like allocator for
video buffers. It reserves a contiguous block of physical memory at boot for
DMA-capable video frame buffers, shared between the ISP, encoder, and AI
engines.

The `binder.ko` module (loaded with `r0size=16 r2size=320`) provides
Android-style IPC between sonia's internal modules. The r0/r2 sizes configure
shared memory region sizes in pages.

## Product variants

The `product.cfg` file maps HWID strings to hardware profiles. Each profile
is a colon-separated field string:

```
CA-HZ4032GB-C:01:03:04:78:56:00:01:10:01:01:04:3E8_3E8:04:00:00:04:00:01:00:00:400
{ product=CA-HZ4032GB-C ViStream=400W VideoOutChip=SOC }
```

This single firmware package supports 30+ camera models in the SD-Taurus
family. The HWID (read from eMMC partition p7 at boot) selects which product
profile to use, configuring:
- Video resolution (`ViStream=200W` for 2MP, `400W` for 4MP, `800` for 8MP)
- Video output chip (`VideoOutChip=SOC` for built-in)
- Sensor type and lens parameters
- Motor driver configuration
- Feature gates (AI, WiFi, 4G, etc.)

## PTZ subsystem

For PTZ cameras like the DH-SD5A425GB-HNR:
- `motorDrv.ko` — stepper/servo motor driver via GPIO
- `mult_len_driver.ko` — zoom lens controller
- `/usr/bin/MultLenCtrlTools` — lens calibration utility
- `LensParams` — per-model lens parameter database

PTZ protocol capabilities are exposed via `ptz.cgi` (HTTP) and `ptz.*` (RPC2).

## Networking

- Wired: built-in Ethernet MAC
- WiFi: optional, via USB dongles — drivers for RTL8192EU, RTL8811AU, AIC8800
- Cellular: optional 4G/5G via USB modules — Qualcomm GobiNet, Fibocom, others
- The romfs includes extensive cellular management tools:
  `3gconfig`, `3gpp`, `4G`, `fibo_*`, `foat_*`, `gmod_*`
