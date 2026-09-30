"""Task-level source dates. Never infer age from crawl time or the site year.

РЕШУ's print view exposes .attr4 (Источник). The methodist page lists real
dated exam variants; their heading and task membership are verified separately.
An older/undated citation takes precedence over a recent re-publication.
"""
import re
from datetime import datetime, timezone

from bs4 import BeautifulSoup


def clean(value):
    return re.sub(r"\s+", " ", value.replace("\xad", "").replace("\u200b", "")).strip()


def source_date(label):
    """Only a full original EGE date is evidence; a year alone is not."""
    label = clean(label)
    if "ЕГЭ" not in label or "ОГЭ" in label:
        return None
    dates = []
    for day, month, year in re.findall(r"(?<!\d)(\d{2})\.(\d{2})\.(20\d{2})(?!\d)", label):
        try:
            dates.append(datetime(int(year), int(month), int(day), tzinfo=timezone.utc))
        except ValueError:
            return None
    # A citation naming an older year but a new full date is not fresh proof.
    if not dates or any(int(y) < min(d.year for d in dates) for y in re.findall(r"\b(20\d{2})\b", label)):
        return None
    return min(dates)


def current_date(value, now=None):
    now = now or datetime.now(timezone.utc)
    try:
        cutoff = now.replace(year=now.year - 1)
    except ValueError:  # 29 February -> 28 February in the previous year
        cutoff = now.replace(year=now.year - 1, day=28)
    return value is not None and cutoff <= value <= now


def evidence(content, url, inherited=None, now=None):
    """Evidence inherited only from a verified official dated variant listing.

    Keep the earliest explicit citation. Any undated citation makes the age
    unknown, even if the same task occurs in this year's official variant.
    """
    now = now or datetime.now(timezone.utc)
    result = {"verified_at": now.isoformat()}
    candidates = []
    if inherited:
        date = source_date(inherited["label"])
        if date:
            candidates.append((date, inherited.get("evidence_url", inherited["url"]), clean(inherited["label"])))
    soup = BeautifulSoup(content, "html.parser")
    for citation in soup.select(".attr4"):
        label = clean(citation.get_text(" ", strip=True))
        # Empty controls aren't citations. Non-empty source without a full
        # date deliberately withholds publication proof.
        if not label:
            continue
        date = source_date(label)
        if date is None:
            return result
        candidates.append((date, url, label))
    if candidates:
        date, proof_url, label = min(candidates, key=lambda c: c[0])
        result.update(published_at=date.isoformat(), date_evidence_url=proof_url,
                      date_evidence=label)
    return result
