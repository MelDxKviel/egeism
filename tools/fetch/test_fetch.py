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
