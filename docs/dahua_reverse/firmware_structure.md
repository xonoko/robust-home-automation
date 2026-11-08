# Firmware Package Structure

## Outer packaging

The firmware ships as a ZIP file:

```
DH_SD-Taurus_MultiLang_PN_Stream3-Normal_V3.200.0000028.6.R.260514.zip
```

Inside the ZIP is a DHSP container (Dahua's proprietary firmware packaging
format). The DHSP wraps a second ZIP whose entries are the individual
partition images plus metadata.

## Extracted images

| File | Size | Partition | Format |
|------|------|-----------|--------|
| `dhboot-min.bin.img` | 33 KB | mini-boot | Raw bootloader, unencrypted |
| `dhboot.bin.img` | 200 KB | U-Boot | uImage-wrapped, unencrypted |
| `dhdtb.bin.img` | 44 KB | dtb | uImage-wrapped device tree blob |
| `kernel.img` | 4.3 MB | kernel | uImage-wrapped, AES-256-CBC encrypted |
| `romfs-x.squashfs.img` | 69 MB | rootfs | uImage-wrapped, AES-256-CBC encrypted |
| `web-x.squashfs.img` | 19 MB | web | uImage-wrapped, AES-256-CBC encrypted |
| `pd-x.squashfs.img` | 5.1 MB | pd | uImage-wrapped, AES-256-CBC encrypted |
| `firmware-x.ext4_aa.img` | 129 MB | firmware (part 1/3) | uImage-wrapped, AES-256-CBC encrypted |
| `firmware-x.ext4_ab.img` | 129 MB | firmware (part 2/3) | uImage-wrapped, AES-256-CBC encrypted |
| `firmware-x.ext4_ac.img` | 67 MB | firmware (part 3/3) | uImage-wrapped, AES-256-CBC encrypted |
| `check.img` | 7.3 KB | — | uImage-wrapped JSON (language/video config) |
| `hwid` | 6.5 KB | — | JSON array of supported HWID strings |
| `sign.img` | 128 B | — | RSA signature blob |
| `Install` | 989 B | — | JSON manifest (burn order, version, device class) |

## Install manifest

```json
{
  "Commands": [
    "burn dhdtb.bin.img dtb",
    "burn dhboot-min.bin.img mini-boot",
    "burn dhboot.bin.img U-Boot",
    "burn pd-x.squashfs.img pd",
    "burn romfs-x.squashfs.img rootfs",
    "burn kernel.img kernel",
    "burn firmware-x.ext4_aa.img firmware",
    "burn firmware-x.ext4_ab.img firmware",
    "burn firmware-x.ext4_ac.img firmware",
    "burn web-x.squashfs.img web"
  ],
  "Devices": [["IPC-HX3XXX", "1.00"]],
  "Version": {"SoftwareVersion": "V3.200.0000028.6.R.260514"},
  "spilteSize": "134220024"
}
```

The `spilteSize` (sic) is the split boundary for the ext4 image — each `_aa`
and `_ab` part is exactly this many bytes (128 MB); `_ac` holds the remainder.
The updater concatenates them into a single ext4 image before writing to flash.

## uImage header

Every `.img` file starts with a 64-byte U-Boot uImage header:

```
Offset  Field
0x00    Magic: 0x27051956
0x04    Header CRC32
0x08    Timestamp
0x0C    Data size (big-endian)
0x10    Load address
0x14    Entry point
0x18    Data CRC32
0x1C    OS (5 = Linux)
0x1D    Architecture (2 = ARM)
0x1E    Image type
0x1F    Compression (0=none, 1=gzip, 2=bzip2)
0x20    Image name (32 bytes, null-terminated)
```

**Important**: the compression byte is at offset **0x1F** (31 decimal),
not 0x18 (24) as some older docs claim. The kernel image uses comp=5
(indicating a custom/vendor compression — actually the zImage self-extractor).

Some images have a *double* uImage header (64+64 = 128 bytes of headers
before the payload). The `strip_uimage()` function in `decrypt_fw.py` handles
both cases, but in practice only single headers were observed in this firmware.
