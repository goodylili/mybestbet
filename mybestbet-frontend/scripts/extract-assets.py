#!/usr/bin/env python3
"""Unpack the mybestbet asset packs into public/ and lib/data/.

The old single-file build inlined every crest, book logo, banner, photo and the
hero video as base64. Here they become ordinary static files, and the lookup
tables the UI needs (team abbreviations and colours, competitions, which logos
are white) go into lib/data/catalog.json.

    python3 scripts/extract-assets.py ../mybestbet-mono/packages/assets

Pass --snapshot to also unpack the sample API responses into
public/mybestbet/snapshot/, which NEXT_PUBLIC_MBB_SNAPSHOT=1 serves instead of
calling the API.
"""
import base64
import glob
import gzip
import io
import json
import pathlib
import re
import sys

from PIL import Image

ROOT = pathlib.Path(__file__).resolve().parent.parent
PUBLIC = ROOT / "public" / "mybestbet"
DATA = ROOT / "lib" / "data"
EXT = {"image/webp": ".webp", "image/avif": ".avif", "image/png": ".png", "image/svg+xml": ".svg", "image/jpeg": ".jpg"}


def slug(key):
    return re.sub(r"[^a-z0-9]+", "-", key.lower()).strip("-") or "x"


USED = set()


def write_uri(uri, folder, name):
    head, payload = uri.split(",", 1)
    mime = head[5:].split(";")[0]
    base, n = slug(name), 2
    path = folder / (base + EXT.get(mime, ".bin"))
    while path in USED:
        path = folder / (base + "-" + str(n) + EXT.get(mime, ".bin"))
        n += 1
    USED.add(path)
    path.parent.mkdir(parents=True, exist_ok=True)
    raw = base64.b64decode(payload)
    path.write_bytes(raw)
    return "/" + str(path.relative_to(ROOT / "public")), raw


def is_light(raw):
    try:
        im = Image.open(io.BytesIO(raw)).convert("RGBA")
    except Exception:
        return False
    im.thumbnail((48, 48))
    px = [p for p in (im.get_flattened_data() if hasattr(im, "get_flattened_data") else im.getdata()) if p[3] > 128]
    return bool(px) and sum(0.3 * p[0] + 0.59 * p[1] + 0.11 * p[2] for p in px) / len(px) >= 235


def packed(path):
    line = pathlib.Path(path).read_text(encoding="utf-8").strip()
    return line.split("|") if line.count("|") == 2 else None


def main(src, snapshot):
    src = pathlib.Path(src).resolve()
    d = json.loads((src / "data.json").read_text(encoding="utf-8"))
    img = dict(d["IMG"])
    for k, v in json.loads((src / "logos" / "all.json").read_text())["src"].items():
        if not k.startswith("L_"):
            img["s_" + k] = v
    teams = d["TEAMS"]
    for n, (uri, _colour) in json.loads((src / "logos" / "extra.json").read_text()).items():
        if n in teams:
            teams[n] = (teams[n] + [None] * 4)[:4]
            img["c_" + n] = uri
            teams[n][3] = "c_" + n
    for f in sorted(glob.glob(str(src / "crest" / "c*.txt"))):
        for line in open(f, encoding="utf-8"):
            line = line.strip()
            if not line:
                continue
            n, _h, b = line.split("|")
            if n in teams:
                teams[n] = (teams[n] + [None] * 4)[:4]
                img["c_" + n] = "data:image/webp;base64," + b
                teams[n][3] = "c_" + n

    paths, light = {}, []
    for k, uri in img.items():
        if not uri.startswith("data:image"):
            continue
        p, raw = write_uri(uri, PUBLIC / "img", k)
        paths[k] = p
        if is_light(raw):
            light.append(k)

    banners = {}
    found = {}
    for f in glob.glob(str(src / "banners" / "*.txt")):
        rec = packed(f)
        if rec:
            found[rec[0]] = rec[2]
    if "SerieA_EN" in found:
        found["SerieA"] = found["SerieA_EN"]
    for code, h in json.loads((src / "banner-map.json").read_text()).items():
        if h in found:
            banners[code], _ = write_uri("data:image/webp;base64," + found[h], PUBLIC / "banners", h)
    for code, k in json.loads((src / "lbanner-map.json").read_text()).items():
        f = src / "lbanners" / (k + ".txt")
        if code in banners or not f.exists():
            continue
        rec = packed(f)
        if rec:
            banners[code], _ = write_uri("data:image/avif;base64," + rec[2], PUBLIC / "banners", k)

    photos = {}
    for f in glob.glob(str(src / "photos" / "*.txt")):
        rec = packed(f)
        if rec:
            photos[rec[0]], _ = write_uri("data:image/avif;base64," + rec[2], PUBLIC / "photos", rec[0])

    video = src / "video" / "herovid.txt"
    hero = None
    if video.exists():
        _n, _h, b = video.read_text().strip().split("|")
        out = PUBLIC / "video" / "hero.webm"
        out.parent.mkdir(parents=True, exist_ok=True)
        out.write_bytes(base64.b64decode(b))
        hero = "/mybestbet/video/hero.webm"

    for name in ["Daniil Medvedev", "Karen Khachanov", "Andrey Rublev", "Aryna Sabalenka", "Ekaterina Alexandrova", "Mirra Andreeva", "Liudmila Samsonova", "Diana Shnaider"]:
        if name in teams and "f_ru" in paths:
            teams[name] = (teams[name] + [None] * 4)[:4]
            teams[name][3] = "f_ru"

    catalog = {
        "teams": {n: {"abbr": t[0], "color": t[1], "logo": t[3] if len(t) > 3 else None} for n, t in teams.items()},
        "comps": [{k: c[k] for k in ("id", "sp", "grp", "name", "mk")} for c in d["COMPS"]],
        "img": paths,
        "light": sorted(light),
        "banners": banners,
        "photos": photos,
        "heroVideo": hero,
    }
    DATA.mkdir(parents=True, exist_ok=True)
    (DATA / "catalog.json").write_text(json.dumps(catalog, ensure_ascii=False, separators=(",", ":")), encoding="utf-8")

    if snapshot:
        snap = json.loads((src / "snapshot.json").read_text())
        out = PUBLIC / "snapshot"
        for key, b64 in snap["files"].items():
            target = out / (key.strip("/") + ".json")
            target.parent.mkdir(parents=True, exist_ok=True)
            target.write_bytes(gzip.decompress(base64.b64decode(b64)))
        (out / "meta.json").write_text(json.dumps({"takenAt": snap["t"]}))

    print(len(paths), "images,", len(banners), "banners,", len(photos), "photos")


if __name__ == "__main__":
    args = [a for a in sys.argv[1:] if not a.startswith("--")]
    main(args[0] if args else "../mybestbet-mono/packages/assets", "--snapshot" in sys.argv)
