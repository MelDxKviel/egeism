"""Discover actual recent EGE tasks via dated official FIPI variants on РЕШУ.

The source is attributed to РЕШУ, not misrepresented as an official FIPI API.
No static task IDs, invented tasks, or current-year copyright heuristics.
"""
import re
import time
from concurrent.futures import ThreadPoolExecutor, TimeoutError
from urllib.parse import parse_qs, urljoin, urlparse

import requests
from bs4 import BeautifulSoup, Tag

import fetch as F
from freshness import clean, current_date, evidence, source_date


def _line(anchor):
    before, after = [], []
    for node in anchor.previous_siblings:
        if getattr(node, "name", None) in ("br", "h3", "center", "p", "div"):
            break
        before.append(node.get_text(" ") if isinstance(node, Tag) else str(node))
    for node in anchor.next_siblings:
        if getattr(node, "name", None) in ("br", "h3", "center", "p", "div"):
            break
        after.append(node.get_text(" ") if isinstance(node, Tag) else str(node))
    return clean(" ".join(list(reversed(before)) + [anchor.get_text(" ")] + after))


def variant_links(content, base, now=None):
    links = []
    soup = BeautifulSoup(content, "html.parser")
    for a in soup.select('a[href*="/test?"]'):
        label = _line(a)
        date = source_date(label)
        url = urljoin(base, a.get("href", ""))
        # Official released FIPI variants, not anonymous teacher tests or mocks.
        if ("ФИПИ" in label and "вариант" in label.lower() and current_date(date, now)
                and urlparse(url).hostname == urlparse(base).hostname):
            links.append({"url": url, "label": label, "evidence_url": base + "/methodist"})
    return list({v["url"]: v for v in links}.values())


def variant_members(content, variant, base):
    soup = BeautifulSoup(content, "html.parser")
    expected = source_date(variant["label"])
    # Some live/print templates omit the variant title. The trusted dated
    # methodist link plus exact task membership is still the source evidence.
    # When a dated FIPI title is present, reject a contradictory date.
    titles = [clean(h.get_text(" ")) for h in soup.select("h2, h3")
              if "ФИПИ" in clean(h.get_text(" "))]
    if any((source_date(title) is not None and source_date(title) != expected)
           or any(int(year) != expected.year for year in re.findall(r"\b20\d{2}\b", title))
           for title in titles):
        return []
    members = []
    for index, block in enumerate(soup.select(".prob_maindiv"), 1):
        link = block.select_one('.prob_nums a[href*="problem?id="]')
        if not link:
            continue
        pid = parse_qs(urlparse(link["href"]).query).get("id", [""])[0]
        if not pid.isdigit():
            continue
        # This is the position in the official exam, not РЕШУ's topic taxonomy.
        position = int(block.get("data-num") or index)
        members.append((pid, position))
    return members


def fetch(subject, number, limit, budget=60.0):
    deadline = time.monotonic() + budget
    base = F._base_url(F.SDAMGIA_SUBJECT[subject])
    session = requests.Session()
    session.headers["User-Agent"] = F.UA

    def get(url):
        if time.monotonic() >= deadline:
            raise TimeoutError()
        response = session.get(url, timeout=min(12, max(.1, deadline - time.monotonic())))
        response.raise_for_status()
        return response.content

    variants = variant_links(get(base + "/methodist"), base)
    candidates = []
    seen = set()
    for variant in variants[:4]:
        for pid, position in variant_members(get(variant["url"] + "&print=true"), variant, base):
            if (number and position != number) or pid in seen:
                continue
            seen.add(pid)
            candidates.append((pid, position, variant))

    def one(candidate):
        pid, position, variant = candidate
        url = base + "/problem?id=" + pid
        try:
            content = get(url + "&print=true")
            parsed = F._parse_problem(content, base)
            if not parsed:
                return None
            _, statement, media, answer = parsed
            raw = F.to_raw_task(subject, pid, url, str(position), statement, media, answer)
            if raw:
                raw["source"].update(evidence(content, url + "&print=true", variant))
            return raw
        except Exception:
            return None

    pool = ThreadPoolExecutor(max_workers=4)
    try:
        futures = [pool.submit(one, c) for c in candidates]
        yielded = 0
        for future in futures:
            try:
                raw = future.result(timeout=max(.1, deadline - time.monotonic()))
            except TimeoutError:
                break
            if raw:
                yield raw
                yielded += 1
                if yielded >= limit:
                    break
    finally:
        pool.shutdown(wait=False, cancel_futures=True)
