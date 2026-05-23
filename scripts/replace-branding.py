#!/usr/bin/env python3
from __future__ import annotations

import argparse
from pathlib import Path

REPLACEMENTS = [
    ("cmd/ds2api-tests", "cmd/tool-gateway-tests"),
    ("cmd/ds2api", "cmd/tool-gateway"),
    ("# DS2API", "# Tool Gateway"),
    ("DS2API runtime", "Tool Gateway runtime"),
    ("DS2API 配置文件示例", "Tool Gateway 配置文件示例"),
    ("DS2API 接口文档", "Tool Gateway 接口文档"),
    ("DS2API API Reference", "Tool Gateway API Reference"),
    ("DS2API 架构与项目结构说明", "Tool Gateway 架构与项目结构说明"),
    ("DS2API Architecture & Project Layout", "Tool Gateway Architecture & Project Layout"),
    ("DS2API 部署指南", "Tool Gateway 部署指南"),
    ("DS2API Deployment Guide", "Tool Gateway Deployment Guide"),
]

SKIP_DIRS = {".git", "node_modules", "dist", "coverage", ".claude", ".tmp"}
TEXT_EXTS = {
    "", ".md", ".txt", ".json", ".yml", ".yaml", ".sh", ".js", ".mjs", ".ts", ".tsx", ".jsx", ".go", ".html"
}


def should_skip(path: Path) -> bool:
    return any(part in SKIP_DIRS for part in path.parts)


def is_text_file(path: Path) -> bool:
    return path.suffix.lower() in TEXT_EXTS or path.name in {"Dockerfile", "LICENSE"}


def main() -> int:
    parser = argparse.ArgumentParser(description="Apply safe Tool Gateway branding replacements")
    parser.add_argument("root", nargs="?", default=".", help="repository root")
    parser.add_argument("--apply", action="store_true", help="write changes instead of dry-run")
    args = parser.parse_args()

    root = Path(args.root).resolve()
    changed = 0

    for path in root.rglob("*"):
        if not path.is_file() or should_skip(path):
            continue
        if not is_text_file(path):
            continue

        try:
            text = path.read_text(encoding="utf-8")
        except Exception:
            continue

        original = text
        for old, new in REPLACEMENTS:
            text = text.replace(old, new)

        if text == original:
            continue

        changed += 1
        rel = path.relative_to(root)
        print(rel)
        if args.apply:
            path.write_text(text, encoding="utf-8")

    print(f"changed_files={changed}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
