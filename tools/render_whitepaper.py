#!/usr/bin/env python3
"""Render docs/whitepaper.md into a deterministic, publication-ready PDF."""

from __future__ import annotations

import html
import os
import re
from pathlib import Path

from pypdf import PdfReader
from reportlab.graphics.shapes import Drawing, Line, Polygon, Rect, String
from reportlab.lib import colors
from reportlab.lib.enums import TA_CENTER
from reportlab.lib.pagesizes import LETTER
from reportlab.lib.styles import ParagraphStyle, getSampleStyleSheet
from reportlab.lib.units import inch
from reportlab.pdfbase.ttfonts import TTFont
from reportlab.pdfbase import pdfmetrics
from reportlab.platypus import (
    KeepTogether,
    ListFlowable,
    ListItem,
    PageBreak,
    Paragraph,
    Preformatted,
    SimpleDocTemplate,
    Spacer,
)

ROOT = Path(__file__).resolve().parents[1]
SOURCE = ROOT / "docs" / "whitepaper.md"
OUTPUT = ROOT / "output" / "pdf" / "typeference-whitepaper.pdf"
os.environ.setdefault("SOURCE_DATE_EPOCH", "1782993600")  # 2026-07-02T12:00:00Z
NAVY = colors.HexColor("#171A2B")
INDIGO = colors.HexColor("#4455AA")
PURPLE = colors.HexColor("#8154B5")
PALE = colors.HexColor("#F1E9FF")
INK = colors.HexColor("#25283A")
MUTED = colors.HexColor("#5B6079")


def register_fonts() -> tuple[str, str, str]:
    candidates = [
        Path("C:/Windows/Fonts/aptos.ttf"),
        Path("C:/Windows/Fonts/arial.ttf"),
    ]
    bolds = [Path("C:/Windows/Fonts/aptos-bold.ttf"), Path("C:/Windows/Fonts/arialbd.ttf")]
    monos = [Path("C:/Windows/Fonts/cascadiamono.ttf"), Path("C:/Windows/Fonts/consola.ttf")]
    body, bold, mono = "Helvetica", "Helvetica-Bold", "Courier"
    for path in candidates:
        if path.exists():
            pdfmetrics.registerFont(TTFont("TFBody", str(path))); body = "TFBody"; break
    for path in bolds:
        if path.exists():
            pdfmetrics.registerFont(TTFont("TFBold", str(path))); bold = "TFBold"; break
    for path in monos:
        if path.exists():
            pdfmetrics.registerFont(TTFont("TFMono", str(path))); mono = "TFMono"; break
    return body, bold, mono


BODY, BOLD, MONO = register_fonts()


def styles():
    base = getSampleStyleSheet()
    return {
        "title": ParagraphStyle("Title", parent=base["Title"], fontName=BOLD, fontSize=31, leading=35, textColor=NAVY, spaceAfter=14),
        "subtitle": ParagraphStyle("Subtitle", parent=base["Normal"], fontName=BODY, fontSize=15, leading=21, textColor=INDIGO, alignment=TA_CENTER, spaceAfter=22),
        "author": ParagraphStyle("Author", parent=base["Normal"], fontName=BODY, fontSize=10, leading=15, textColor=MUTED, alignment=TA_CENTER, spaceBefore=8),
        "h1": ParagraphStyle("H1", parent=base["Heading1"], fontName=BOLD, fontSize=19, leading=23, textColor=NAVY, spaceBefore=18, spaceAfter=9, keepWithNext=True),
        "h2": ParagraphStyle("H2", parent=base["Heading2"], fontName=BOLD, fontSize=13, leading=17, textColor=INDIGO, spaceBefore=12, spaceAfter=7, keepWithNext=True),
        "body": ParagraphStyle("Body", parent=base["BodyText"], fontName=BODY, fontSize=9.6, leading=14.2, textColor=INK, spaceAfter=7, allowWidows=0, allowOrphans=0),
        "bullet": ParagraphStyle("Bullet", parent=base["BodyText"], fontName=BODY, fontSize=9.3, leading=13.5, leftIndent=14, textColor=INK),
        "code": ParagraphStyle("Code", parent=base["Code"], fontName=MONO, fontSize=7.7, leading=10.2, leftIndent=10, rightIndent=10, borderColor=colors.HexColor("#D8DBE8"), borderWidth=0.5, borderPadding=8, backColor=colors.HexColor("#F7F8FC"), spaceAfter=9),
        "caption": ParagraphStyle("Caption", parent=base["Normal"], fontName=BODY, fontSize=8, leading=11, textColor=MUTED, alignment=TA_CENTER, spaceAfter=10),
    }


S = styles()


def inline(text: str) -> str:
    value = html.escape(text, quote=False)
    value = re.sub(r"`([^`]+)`", r'<font name="TFMono">\1</font>' if MONO == "TFMono" else r'<font name="Courier">\1</font>', value)
    value = re.sub(r"\*\*([^*]+)\*\*", r"<b>\1</b>", value)
    return value


def arrow(d: Drawing, x1, y1, x2, y2, color=MUTED):
    d.add(Line(x1, y1, x2, y2, strokeColor=color, strokeWidth=1.4))
    d.add(Polygon([x2, y2, x2 - 7, y2 + 3.5, x2 - 7, y2 - 3.5], fillColor=color, strokeColor=color))


def node(d, x, y, w, h, title, subtitle="", fill=colors.HexColor("#F7F8FC"), stroke=INDIGO, white=False):
    d.add(Rect(x, y, w, h, rx=7, ry=7, fillColor=fill, strokeColor=stroke, strokeWidth=1.4))
    tc = colors.white if white else NAVY
    sc = colors.white if white else MUTED
    d.add(String(x + w / 2, y + h - 20, title, textAnchor="middle", fontName=BOLD, fontSize=10, fillColor=tc))
    if subtitle: d.add(String(x + w / 2, y + 13, subtitle, textAnchor="middle", fontName=BODY, fontSize=7.2, fillColor=sc))


def hierarchy():
    d = Drawing(470, 238)
    node(d, 165, 190, 140, 38, "system/object", "mechanics only", NAVY, NAVY, True)
    node(d, 155, 125, 160, 42, "enterprise-agent", "organization-owned", colors.HexColor("#E9ECFF"))
    node(d, 25, 60, 160, 42, "person-agent", "human-facing domain")
    node(d, 285, 60, 160, 42, "repo-agent", "repository domain")
    node(d, 25, 0, 160, 38, "executive-assistant", fill=PALE, stroke=PURPLE)
    node(d, 285, 0, 160, 38, "payments-repo-agent", fill=PALE, stroke=PURPLE)
    for x1, y1, x2, y2 in [(235,190,235,167),(235,125,105,102),(235,125,365,102),(105,60,105,38),(365,60,365,38)]:
        d.add(Line(x1,y1,x2,y2,strokeColor=MUTED,strokeWidth=1.2))
    return d


def pipeline():
    d = Drawing(470, 165)
    node(d, 0, 60, 90, 48, "Typed YAML", "agents + skills")
    arrow(d, 90, 84, 120, 84)
    node(d, 120, 60, 90, 48, "Validate", "contracts + paths")
    arrow(d, 210, 84, 240, 84)
    node(d, 240, 55, 105, 58, "Resolved IR", "behavior + provenance", NAVY, NAVY, True)
    for y, label in [(125,"Codex"),(72,"Copilot"),(19,"Cursor + MCP")]:
        arrow(d, 345, 84, 375, y+18)
        node(d, 375, y, 95, 36, label, fill=PALE, stroke=PURPLE)
    return d


def interoperability_stack():
    d = Drawing(470, 220)
    node(d, 150, 172, 170, 40, "TypeFerence source", "canonical + governed", NAVY, NAVY, True)
    targets = [(0, "Codex bundle", "target-specific"), (125, "Copilot bundle", "target-specific"),
               (250, "Cursor bundle", "target-specific"), (375, "MCP card", "callable protocol")]
    catalog_ports = [125, 200, 270, 345]
    for (x, label, subtitle), port in zip(targets, catalog_ports):
        arrow(d, 235, 172, x + 47, 142)
        node(d, x, 102, 95, 40, label, subtitle, colors.HexColor("#E9ECFF"))
        arrow(d, x + 47, 102, port, 78)
    node(d, 75, 38, 320, 40, "ARD catalog", "source entry + separately versioned target entries", PALE, PURPLE)
    arrow(d, 235, 38, 235, 23)
    d.add(String(235, 9, "consumer selects a compatible artifact or callable protocol", textAnchor="middle", fontName=BODY, fontSize=7.2, fillColor=MUTED))
    return d


def dispatch():
    d = Drawing(470, 150)
    node(d, 0, 48, 130, 55, "MCP call", "derived skill name")
    arrow(d, 130, 75, 165, 75)
    node(d, 165, 40, 140, 70, "Resolve contract", "nearest compatible override", NAVY, NAVY, True)
    arrow(d, 305, 75, 340, 75)
    node(d, 340, 20, 130, 110, "Invocation package", "args + context + provenance", PALE, PURPLE)
    return d


def cross_agent():
    d = Drawing(470, 168)
    node(d, 0, 92, 130, 52, "Executive assistant", "owns the brief")
    node(d, 170, 92, 130, 52, "TypeFerence MCP", "typed boundary", PALE, PURPLE)
    node(d, 340, 92, 130, 52, "Repo agent", "owns evidence")
    arrow(d,130,118,170,118); arrow(d,300,118,340,118)
    arrow(d,340,96,300,96); arrow(d,170,96,130,96)
    node(d, 105, 10, 260, 50, "Evidence-backed brief", "distinct responsibilities, shared coherence", PALE, PURPLE)
    arrow(d,65,92,105,35)
    return d


DIAGRAMS = {
    "type-hierarchy.svg": hierarchy,
    "compiler-pipeline.svg": pipeline,
    "interoperability-stack.svg": interoperability_stack,
    "dispatch.svg": dispatch,
    "cross-agent.svg": cross_agent,
}


def parse_markdown(text: str):
    lines = text.splitlines()
    story = []
    paragraph = []
    bullets = []
    code = []
    in_code = False
    first_h1 = True
    cover_subtitle = False
    cover_complete = False

    def flush_paragraph():
        if paragraph:
            story.append(Paragraph(inline(" ".join(paragraph)), S["body"])); paragraph.clear()

    def flush_bullets():
        if bullets:
            items = [ListItem(Paragraph(inline(x), S["bullet"]), leftIndent=12) for x in bullets]
            story.append(ListFlowable(items, bulletType="bullet", start="circle", leftIndent=18, spaceAfter=8)); bullets.clear()

    for line in lines:
        if line.startswith("```"):
            flush_paragraph(); flush_bullets()
            if in_code:
                story.append(Preformatted("\n".join(code), S["code"])); code.clear()
            in_code = not in_code; continue
        if in_code: code.append(line); continue
        image = re.fullmatch(r"!\[([^]]+)\]\(assets/([^)]+)\)", line.strip())
        if image:
            flush_paragraph(); flush_bullets()
            drawing = DIAGRAMS[image.group(2)]()
            story.append(KeepTogether([drawing, Paragraph("Figure: " + html.escape(image.group(1)), S["caption"])])); continue
        if line.startswith("# "):
            flush_paragraph(); flush_bullets()
            if first_h1:
                story.extend([Spacer(1, 0.9 * inch), Paragraph(inline(line[2:]), S["title"])]); first_h1 = False
            else: story.extend([PageBreak(), Paragraph(inline(line[2:]), S["h1"])])
            continue
        if line.startswith("## "):
            flush_paragraph(); flush_bullets()
            if not cover_subtitle:
                story.append(Paragraph(inline(line[3:]), S["subtitle"])); cover_subtitle = True
            else: story.append(Paragraph(inline(line[3:]), S["h1"]))
            continue
        if line.startswith("### "):
            flush_paragraph(); flush_bullets(); story.append(Paragraph(inline(line[4:]), S["h2"])); continue
        if line.startswith("- "):
            flush_paragraph(); bullets.append(line[2:]); continue
        if re.match(r"\d+\. ", line):
            flush_paragraph(); bullets.append(line); continue
        if not line.strip():
            flush_paragraph(); flush_bullets(); continue
        if cover_subtitle and not cover_complete:
            story.append(Paragraph(inline(line), S["author"])); story.append(Spacer(1, 2.4 * inch)); story.append(PageBreak()); cover_complete = True
        else: paragraph.append(line.strip())
    flush_paragraph(); flush_bullets()
    return story


def decorate(canvas, doc):
    canvas.saveState()
    page = canvas.getPageNumber()
    canvas.setStrokeColor(colors.HexColor("#D8DBE8")); canvas.line(0.72*inch, 0.55*inch, 7.78*inch, 0.55*inch)
    canvas.setFont(BODY, 7.5); canvas.setFillColor(MUTED)
    canvas.drawString(0.72*inch, 0.36*inch, "TypeFerence - typed coherence for portable organizational agents")
    canvas.drawRightString(7.78*inch, 0.36*inch, str(page))
    canvas.restoreState()


def main():
    OUTPUT.parent.mkdir(parents=True, exist_ok=True)
    doc = SimpleDocTemplate(str(OUTPUT), pagesize=LETTER, rightMargin=0.72*inch, leftMargin=0.72*inch, topMargin=0.68*inch, bottomMargin=0.72*inch,
                            title="TypeFerence: A typed coherence layer for portable organizational agents", author="TypeFerence contributors",
                            invariant=1)
    doc.build(parse_markdown(SOURCE.read_text(encoding="utf-8")), onFirstPage=decorate, onLaterPages=decorate)
    reader = PdfReader(str(OUTPUT))
    if len(reader.pages) < 6: raise RuntimeError("Whitepaper unexpectedly short")
    extracted = "".join(page.extract_text() or "" for page in reader.pages)
    for phrase in ("TypeFerence", "system/object", "deterministic compiler", "Conclusion"):
        if phrase not in extracted: raise RuntimeError(f"Missing PDF text: {phrase}")
    print(f"Rendered {OUTPUT} ({len(reader.pages)} pages)")


if __name__ == "__main__": main()
