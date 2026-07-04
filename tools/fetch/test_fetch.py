"""Regression tests for the РЕШУ condition extractor (`_parse_problem`).

Offline by design — the fixtures below reproduce the load-bearing shape of a real
РЕШУ /problem page (verified live against rus/math/soc, 2026-07), so the parser is
pinned without depending on the network or the sdamgia lib. The bug these guard
against: a русский задание page carries a collapsible «Что проверяется…» theory
card (hidden via display:none) as the FIRST pbody, so grabbing `pbodies[0]` dumped
the whole methodology — codifier notes, downloadable files, a worked example, the
ALGORITHM — into the statement instead of the actual question.

The tell РЕШУ gives us: the CONDITION (statement + «самостоятельно подберите…»
instruction) is wrapped in a `div.nobreak`; the «Пояснение/Решение» is a bare
pbody outside it; theory cards are display:none. See `_condition_pbodies`.
"""
import fetch as F
import openfipi as OF
from bs4 import BeautifulSoup

# A русский задание-1 page: hidden theory card + passage + instruction (all under
# nobreak) + a bare solution pbody + a second (bare, hidden) theory + the answer.
RUS_HTML = """
<div class="prob_maindiv" id="maindiv1">
  <div class="nobreak">
    <span class="prob_nums">Тип 1 &#8470; 50643</span>
    <div style="display:none"><div align="justify"><div class="pbody">
      Задание 1. Что проверяется и что нужно знать? Элементы содержания по
      «Кодификатору». Файлы для скачивания. АЛГОРИТМ. Шаг 1. Прочитай текст целиком.
    </div></div></div>
    <div class="probtext" id="text1"><div><div class="pbody">
      Прочитайте текст и выполните задание. Существует Вселенная &lt;...&gt; язык.
    </div></div></div>
    <div class="pbody">Самостоятельно подберите разделительный союз, который должен
      стоять на месте пропуска. Запишите этот союз.</div>
  </div>
  <div class="pbody">Пояснение. Разделительный союз выражает выбор. Ответ: или.</div>
  <div style="display:none"><div class="pbody">Задание 1. Что проверяется?</div></div>
  <div class="answer">Ответ: или|либо</div>
</div>
"""

# A math/обществознание shape: single nobreak condition (with an inline tex
# formula) + a bare solution pbody that ALSO carries a tex formula and the answer.
MATH_HTML = """
<div class="prob_maindiv" id="maindiv1">
  <div class="nobreak">
    <span class="prob_nums">Тип 1 &#8470; 27238</span>
    <div class="pbody">В треугольнике ABC угол C равен 90°,
      <img class="tex" src="/formula/ac.png" alt="AC=4,8"> Найдите AB.</div>
  </div>
  <div class="pbody">Решение. Найдём косинус угла A:
    <img class="tex" src="/formula/sol.png" alt="cosA"> Ответ: 5.</div>
  <div class="answer">Ответ: 5</div>
</div>
"""


def _parse(html):
    return F._parse_problem(html.encode("utf-8"), "https://rus-ege.sdamgia.ru")


def test_rus_drops_theory_and_solution_keeps_instruction():
    topic, statement, media, answer = _parse(RUS_HTML)
    assert topic == "1"
    assert answer == "или|либо"
    # the actual condition survives, in reading order (passage then instruction)
    assert "Прочитайте текст" in statement
    assert "Самостоятельно подберите разделительный союз" in statement
    assert statement.index("Прочитайте") < statement.index("Самостоятельно")
    # none of the theory-card boilerplate leaks in
    for junk in ("Что проверяется", "Кодификатору", "Файлы для скачивания", "АЛГОРИТМ"):
        assert junk not in statement, junk
    # nor the solution / explanation
    assert "Пояснение" not in statement


def test_math_keeps_inline_formula_drops_solution():
    _, statement, media, answer = _parse(MATH_HTML)
    assert answer == "5"
    assert "Найдите AB" in statement
    assert "Решение" not in statement  # solution pbody (outside nobreak) excluded
    # the condition's inline formula is kept as a placeholder + inline media…
    assert "⟦img:0⟧" in statement
    assert media and media[0]["inline"] and media[0]["url"].endswith("ac.png")
    # …and the SOLUTION's formula is NOT pulled into the media list
    assert all(not m["url"].endswith("sol.png") for m in media)


def test_fallback_to_first_visible_pbody_when_no_nobreak():
    # If РЕШУ ever drops the nobreak wrapper, degrade to the first VISIBLE pbody
    # (never the hidden theory card) rather than emitting an empty statement.
    html = """
    <div class="prob_maindiv">
      <div style="display:none"><div class="pbody">Что проверяется?</div></div>
      <div class="pbody">Настоящее условие задания.</div>
      <div class="pbody">Пояснение. Решение.</div>
      <div class="answer">Ответ: 7</div>
    </div>"""
    _, statement, _, answer = _parse(html)
    assert statement == "Настоящее условие задания."
    assert answer == "7"


# --- drill (number-targeted) id discovery ------------------------------------

# The shape sdamgia's get_catalog returns: topics (= задания) with categories.
CATALOG = [
    {"topic_id": "1", "topic_name": "Задание 1", "categories": [
        {"category_id": "c11", "category_name": "a"},
        {"category_id": "c12", "category_name": "b"},
    ]},
    {"topic_id": "№ 2", "topic_name": "Задание 2", "categories": [
        {"category_id": "c21", "category_name": "c"},
    ]},
    {"topic_id": "C4", "topic_name": "Часть 2", "categories": [
        {"category_id": "part2", "category_name": "d"},
    ]},
]


def test_topic_categories_targets_one_number():
    # A drill for задание 1 must draw from ALL of topic 1's categories — and
    # nothing else — so the whole pull budget fills the drill pool.
    assert F._topic_categories(CATALOG, 1) == ["c11", "c12"]
    assert F._topic_categories(CATALOG, 2) == ["c21"]  # «№ 2» label still matches


def test_topic_categories_falls_back_when_unmatched():
    # Unknown/part-2 numbers → [] so fetch() falls back to the all-numbers spread
    # (the server-side number filter still applies).
    assert F._topic_categories(CATALOG, 27) == []
    assert F._topic_categories(None, 3) == []


# --- openfipi table normalization -------------------------------------------

# The load-bearing shape of a ФИПИ задание-1 distance matrix on openfipi: the
# top-left corner is a colspan=2/rowspan=2 merged cell, the «Номер пункта»
# banner spans all value columns, the vertical «Номер пункта» label spans the
# data rows, and every row carries a stray empty <td> at the right edge. Read
# naively this shifts every row differently — the mangled grid this pins against.
MATRIX_HTML = """
<table>
  <tr><td colspan="2" rowspan="2"></td><td colspan="8">Номер пункта</td><td rowspan="2"></td></tr>
  <tr><td>1</td><td>2</td><td>3</td><td>4</td><td>5</td><td>6</td><td>7</td><td>8</td></tr>
  <tr><td rowspan="2">Номер пункта</td><td>1</td>
      <td></td><td>8</td><td></td><td></td><td></td><td></td><td>1</td><td>3</td><td></td></tr>
  <tr><td>2</td><td>8</td><td></td><td></td><td></td><td>74</td><td></td><td></td><td></td><td></td></tr>
</table>
"""


def test_openfipi_matrix_collapses_to_compact_corner_grid():
    table = BeautifulSoup(MATRIX_HTML, "html.parser").find("table")
    md = OF._table_to_markdown(table)
    assert md is not None
    lines = md.strip().splitlines()
    rows = [[c.strip() for c in ln.strip().strip("|").split("|")] for ln in lines]
    header, data1, data2 = rows[0], rows[2], rows[3]  # rows[1] = --- rule

    # The decorative «Номер пункта» banner row and label column are collapsed:
    # what remains is the compact corner matrix (empty corner + column numbers).
    assert "Номер пункта" not in {c for r in rows for c in r}
    assert header[0] == "" and header[1:9] == ["1", "2", "3", "4", "5", "6", "7", "8"]

    # Row П1: index in the first cell, values under their own column headers.
    assert data1[0] == "1"
    assert data1[header.index("2")] == "8"
    assert data1[header.index("7")] == "1"
    assert data1[header.index("8")] == "3"

    # Row П2 stays symmetric with П1.
    assert data2[0] == "2"
    assert data2[header.index("1")] == "8"
    assert data2[header.index("5")] == "74"

    # Trailing empty column trimmed: a clean 9-wide rectangle (corner + 8).
    assert all(len(r) == 9 for r in rows)
