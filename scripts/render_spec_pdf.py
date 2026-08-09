#!/usr/bin/env python3
"""Render the taxi service backend spec markdown into a KarryGo-styled PDF."""

import re
import sys

from reportlab.lib import colors
from reportlab.lib.enums import TA_LEFT
from reportlab.lib.pagesizes import letter
from reportlab.lib.styles import ParagraphStyle
from reportlab.lib.units import inch
from reportlab.pdfbase import pdfmetrics
from reportlab.pdfbase.ttfonts import TTFont
from reportlab.platypus import (
    BaseDocTemplate, Frame, HRFlowable, KeepTogether, PageTemplate,
    Paragraph, Preformatted, Spacer, Table, TableStyle,
)

# ── Fonts ────────────────────────────────────────────────────────────────────
S = "/System/Library/Fonts/Supplemental/"
pdfmetrics.registerFont(TTFont("KG",  S + "Arial.ttf"))
pdfmetrics.registerFont(TTFont("KGB", S + "Arial Bold.ttf"))
pdfmetrics.registerFont(TTFont("KGI", S + "Arial Italic.ttf"))
pdfmetrics.registerFont(TTFont("KGM", S + "Courier New.ttf"))
pdfmetrics.registerFont(TTFont("KGMB", S + "Courier New Bold.ttf"))
pdfmetrics.registerFontFamily("KG", normal="KG", bold="KGB", italic="KGI")

# ── Palette ──────────────────────────────────────────────────────────────────
GREEN      = colors.HexColor("#17663C")
GREEN_DK   = colors.HexColor("#0F4A2B")
RULE_NAVY  = colors.HexColor("#23405C")
INK        = colors.HexColor("#2B2B2B")
INK_SOFT   = colors.HexColor("#5A6B7B")
FOOT       = colors.HexColor("#7A7A7A")
TBL_GRID   = colors.HexColor("#D6E2DA")
TBL_ALT    = colors.HexColor("#F4F8F5")
CODE_BG    = colors.HexColor("#F5F7F6")
CODE_BD    = colors.HexColor("#DCE5E0")

PAGE_W, PAGE_H = letter
LM, RM = 0.85 * inch, 0.85 * inch
TM, BM = 0.95 * inch, 0.80 * inch
CONTENT_W = PAGE_W - LM - RM

RUNNING_HEAD_R = "Taxi Service Backend Requirements"

# ── Styles ───────────────────────────────────────────────────────────────────
def ps(name, **kw):
    kw.setdefault("fontName", "KG")
    kw.setdefault("textColor", INK)
    kw.setdefault("alignment", TA_LEFT)
    return ParagraphStyle(name, **kw)

ST = {
    "title":    ps("title", fontName="KGB", fontSize=22, leading=26,
                   textColor=GREEN, spaceAfter=6),
    "subtitle": ps("subtitle", fontName="KGI", fontSize=10, leading=14,
                   textColor=INK_SOFT, spaceAfter=2),
    "h1":       ps("h1", fontName="KGB", fontSize=13.5, leading=17,
                   textColor=GREEN, spaceBefore=16, spaceAfter=7),
    "h2":       ps("h2", fontName="KGB", fontSize=10.5, leading=14,
                   textColor=GREEN_DK, spaceBefore=11, spaceAfter=5),
    "label":    ps("label", fontName="KGB", fontSize=8.6, leading=12,
                   textColor=GREEN, spaceBefore=7, spaceAfter=2),
    "body":     ps("body", fontSize=9.3, leading=13.4, spaceAfter=6),
    "bullet":   ps("bullet", fontSize=9.3, leading=13.2,
                   leftIndent=14, firstLineIndent=-9, spaceAfter=3.5),
    "th":       ps("th", fontName="KGB", fontSize=8.2, leading=11,
                   textColor=colors.white),
    "td":       ps("td", fontSize=8.2, leading=11),
    "code":     ParagraphStyle("code", fontName="KGM", fontSize=7.3, leading=9.6,
                               textColor=INK, leftIndent=9, rightIndent=6,
                               spaceBefore=8, spaceAfter=11),
}


def esc(t):
    return t.replace("&", "&amp;").replace("<", "&lt;").replace(">", "&gt;")


def inline(t, code_size="8.1"):
    """Markdown inline -> reportlab markup."""
    t = esc(t)
    t = re.sub(
        r"`([^`]+)`",
        lambda m: '<font face="KGM" size="%s" color="#1F4D33">%s</font>'
                  % (code_size, m.group(1)),
        t,
    )
    t = re.sub(r"\*\*([^*]+)\*\*", r"<b>\1</b>", t)
    return t


# ── Markdown -> flowables ────────────────────────────────────────────────────
def split_row(line):
    return [c.strip() for c in line.strip().strip("|").split("|")]


def build_table(header, rows):
    ncol = len(header)
    # weight columns by the longest cell so wide prose columns get more room
    weights = []
    for i in range(ncol):
        cells = [header[i]] + [r[i] for r in rows if i < len(r)]
        longest = max([len(c) for c in cells] or [1])
        # longest unbreakable run drives the minimum sensible width
        longest_tok = max(
            [len(w) for c in cells for w in re.split(r"[\s_\-/.]", c)] or [1]
        )
        weights.append(max(longest, 6) ** 0.72 + longest_tok * 0.55)
    total = sum(weights)
    widths = [CONTENT_W * w / total for w in weights]
    # enforce a sane minimum, then re-normalise
    mn = 0.42 * inch
    widths = [max(w, mn) for w in widths]
    scale = CONTENT_W / sum(widths)
    widths = [w * scale for w in widths]

    data = [[Paragraph(inline(c, "7.7"), ST["th"]) for c in header]]
    for r in rows:
        r = (r + [""] * ncol)[:ncol]
        data.append([Paragraph(inline(c, "7.7"), ST["td"]) for c in r])

    t = Table(data, colWidths=widths, repeatRows=1, hAlign="LEFT")
    style = [
        ("BACKGROUND", (0, 0), (-1, 0), GREEN),
        ("GRID", (0, 0), (-1, -1), 0.5, TBL_GRID),
        ("LINEBELOW", (0, 0), (-1, 0), 0.8, GREEN),
        ("VALIGN", (0, 0), (-1, -1), "TOP"),
        ("LEFTPADDING", (0, 0), (-1, -1), 6),
        ("RIGHTPADDING", (0, 0), (-1, -1), 6),
        ("TOPPADDING", (0, 0), (-1, -1), 5),
        ("BOTTOMPADDING", (0, 0), (-1, -1), 5),
    ]
    for i in range(1, len(data)):
        if i % 2 == 0:
            style.append(("BACKGROUND", (0, i), (-1, i), TBL_ALT))
    t.setStyle(TableStyle(style))
    return t


MAX_CODE_LINES = 32   # smaller panels pack into part-pages instead of jumping


def _panel(chunk):
    """One code panel. A Table renders its fill reliably; Preformatted is a
    bare Flowable whose style backColor is never painted."""
    pre = Preformatted("\n".join(chunk), ST["code"])
    t = Table([[pre]], colWidths=[CONTENT_W], hAlign="LEFT")
    t.setStyle(TableStyle([
        ("BACKGROUND", (0, 0), (-1, -1), CODE_BG),
        ("BOX", (0, 0), (-1, -1), 0.5, CODE_BD),
        ("LINEBEFORE", (0, 0), (0, -1), 1.8, GREEN),
        ("LEFTPADDING", (0, 0), (-1, -1), 9),
        ("RIGHTPADDING", (0, 0), (-1, -1), 8),
        ("TOPPADDING", (0, 0), (-1, -1), 7),
        ("BOTTOMPADDING", (0, 0), (-1, -1), 7),
    ]))
    return t


def code_block(lines):
    # hard-wrap over-long lines so nothing runs off the page
    out = []
    for ln in "\n".join(lines).rstrip().split("\n"):
        while len(ln) > 104:
            out.append(ln[:104])
            ln = "    " + ln[104:]
        out.append(ln)

    # A Table cannot split across pages, so break long listings into
    # page-sized panels, preferring blank lines as the seam.
    panels, start = [], 0
    while len(out) - start > MAX_CODE_LINES:
        window = out[start:start + MAX_CODE_LINES]
        cut = max((i for i, l in enumerate(window) if not l.strip()
                   and i > MAX_CODE_LINES // 2), default=MAX_CODE_LINES)
        panels.append(_panel(out[start:start + cut]))
        start += cut
        while start < len(out) and not out[start].strip():
            start += 1
    panels.append(_panel(out[start:]))
    return panels


def render(md):
    lines = md.split("\n")
    flow = []
    i = 0
    seen_title = False

    while i < len(lines):
        ln = lines[i]
        stripped = ln.strip()

        # fenced code
        if stripped.startswith("```"):
            i += 1
            buf = []
            while i < len(lines) and not lines[i].strip().startswith("```"):
                buf.append(lines[i])
                i += 1
            i += 1
            flow.append(Spacer(1, 3))
            flow.extend(code_block(buf))
            flow.append(Spacer(1, 8))
            continue

        # table
        if stripped.startswith("|") and i + 1 < len(lines) and \
           re.match(r"^\|[\s:\-|]+\|$", lines[i + 1].strip()):
            header = split_row(stripped)
            i += 2
            rows = []
            while i < len(lines) and lines[i].strip().startswith("|"):
                rows.append(split_row(lines[i]))
                i += 1
            flow.append(Spacer(1, 3))
            flow.append(build_table(header, rows))
            flow.append(Spacer(1, 9))
            continue

        # horizontal rule
        if stripped in ("---", "***", "___"):
            i += 1
            continue

        # headings
        if stripped.startswith("# ") and not seen_title:
            seen_title = True
            flow.append(Paragraph(inline(stripped[2:]), ST["title"]))
            i += 1
            # subtitle = next non-blank plain line
            j = i
            while j < len(lines) and not lines[j].strip():
                j += 1
            if j < len(lines) and lines[j].strip() and \
               not lines[j].strip().startswith(("#", "-", "|", "`")):
                flow.append(Paragraph(inline(lines[j].strip()), ST["subtitle"]))
                i = j + 1
            flow.append(Spacer(1, 5))
            flow.append(HRFlowable(width="100%", thickness=1.1, color=RULE_NAVY,
                                   spaceBefore=1, spaceAfter=4))
            continue

        if stripped.startswith("## "):
            flow.append(Paragraph(inline(stripped[3:]), ST["h1"]))
            i += 1
            continue

        if stripped.startswith("### "):
            flow.append(Paragraph(inline(stripped[4:]), ST["h2"]))
            i += 1
            continue

        # standalone bold line -> micro label (Current position / Required behaviour)
        m = re.fullmatch(r"\*\*(.+?)\*\*", stripped)
        if m:
            flow.append(Paragraph(esc(m.group(1)), ST["label"]))
            i += 1
            continue

        # bullets (with continuation lines)
        if re.match(r"^[-*] ", stripped):
            text = stripped[2:]
            i += 1
            while i < len(lines):
                nxt = lines[i]
                if nxt.strip() and nxt.startswith(("  ", "\t")) and \
                   not re.match(r"^\s*[-*] ", nxt) and \
                   not nxt.strip().startswith(("|", "```")):
                    text += " " + nxt.strip()
                    i += 1
                else:
                    break
            flow.append(Paragraph("\u2022&nbsp;&nbsp;" + inline(text), ST["bullet"]))
            continue

        # blank
        if not stripped:
            i += 1
            continue

        # paragraph
        buf = [stripped]
        i += 1
        while i < len(lines) and lines[i].strip() and \
                not re.match(r"^\s*([-*#|>]|```)", lines[i]):
            buf.append(lines[i].strip())
            i += 1
        flow.append(Paragraph(inline(" ".join(buf)), ST["body"]))

    return flow


# ── Page furniture ───────────────────────────────────────────────────────────
def decorate(canvas, doc):
    canvas.saveState()
    y = PAGE_H - 0.55 * inch
    canvas.setFont("KGB", 8)
    canvas.setFillColor(GREEN)
    canvas.drawString(LM, y, "KarryGo")
    w = canvas.stringWidth("KarryGo", "KGB", 8)
    canvas.setFillColor(GREEN_DK)
    canvas.drawString(LM + w, y, "  |  " + RUNNING_HEAD_R)

    canvas.setStrokeColor(TBL_GRID)
    canvas.setLineWidth(0.5)
    canvas.line(LM, y - 5, PAGE_W - RM, y - 5)

    canvas.setFont("KG", 8)
    canvas.setFillColor(FOOT)
    canvas.drawRightString(PAGE_W - RM, 0.5 * inch, f"Page {doc.page}")
    canvas.restoreState()


def main(src, dst):
    with open(src) as f:
        md = f.read()

    doc = BaseDocTemplate(
        dst, pagesize=letter,
        leftMargin=LM, rightMargin=RM, topMargin=TM, bottomMargin=BM,
        title="Taxi Service - Backend Requirements",
        author="KarryGo / Cosmicforge Logistics",
        subject="Backend implementation requirements for driver-taxi-service",
    )
    frame = Frame(LM, BM, CONTENT_W, PAGE_H - TM - BM, id="body",
                  leftPadding=0, rightPadding=0, topPadding=0, bottomPadding=0)
    doc.addPageTemplates([PageTemplate(id="main", frames=[frame],
                                       onPage=decorate)])
    doc.build(render(md))
    print("wrote", dst)


if __name__ == "__main__":
    main(sys.argv[1], sys.argv[2])
