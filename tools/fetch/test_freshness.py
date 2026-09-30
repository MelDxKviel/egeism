from datetime import datetime, timezone

import recent
from freshness import current_date, evidence, source_date

NOW = datetime(2026, 9, 30, 12, tzinfo=timezone.utc)
VARIANT = {"url": "https://math-ege.sdamgia.ru/test?id=90702516",
           "label": "ЕГЭ по математике 27.03.2026. Досрочная волна. Вариант ФИПИ."}


def test_no_import_time_copyright_or_year_only_as_original_date():
    for label in ["ЕГЭ 2026", "© РЕШУ ЕГЭ 2026", "Обновлено 27.03.2026", "ОГЭ 27.03.2026"]:
        assert source_date(label) is None
    assert "published_at" not in evidence('<footer>© ЕГЭ 2026</footer>', VARIANT["url"], now=NOW)


def test_reused_task_keeps_earliest_source_even_in_current_official_variant():
    # Actual verified live shape: task 699087 occurs in 2026 but was used in 2021.
    result = evidence('<div class="attr4">Источник: ЕГЭ. Основная волна 07.06.2021. Урал</div>',
                      "https://math-ege.sdamgia.ru/problem?id=699087&print=true", VARIANT, NOW)
    assert result["published_at"] == "2021-06-07T00:00:00+00:00"
    assert not current_date(datetime.fromisoformat(result["published_at"]), NOW)


def test_undated_older_citation_prevents_rejuvenation():
    result = evidence('<div class="attr4">Демонстрационная версия ЕГЭ—2023</div>',
                      VARIANT["url"], VARIANT, NOW)
    assert "published_at" not in result


def test_fresh_verified_membership_and_future_boundaries():
    result = evidence('<div class="prob_maindiv"></div>', VARIANT["url"], VARIANT, NOW)
    assert result["published_at"] == "2026-03-27T00:00:00+00:00"
    assert result["verified_at"] == NOW.isoformat()
    assert current_date(datetime.fromisoformat(result["published_at"]), NOW)
    assert not current_date(datetime(2027, 1, 1, tzinfo=timezone.utc), NOW)
    assert not current_date(datetime(2025, 9, 30, 11, tzinfo=timezone.utc), NOW)


def test_discover_only_recent_official_variants_not_demo_year_or_teacher_mock():
    html = '''<div>ЕГЭ по математике 27.03.2026. Досрочная волна.
      <a href="/test?id=1">Вариант</a> ФИПИ.<br>
      ЕГЭ по математике 28.03.2025. <a href="/test?id=2">Вариант</a> ФИПИ.<br>
      <a href="/test?id=3">Демоверсия</a> ЕГЭ 2026 года.<br>
      Тренировочный ЕГЭ 27.03.2026. <a href="/test?id=4">Вариант</a> учителя.
    </div>'''
    assert [v["url"] for v in recent.variant_links(html, "https://math-ege.sdamgia.ru", NOW)] == [
        "https://math-ege.sdamgia.ru/test?id=1"]


def test_original_exam_position_is_not_reshu_topic_number():
    html = '''<h3>ЕГЭ 2026 (ФИПИ)</h3><div class="prob_maindiv" data-num="4">
        <span class="prob_nums">Тип 5 № <a href="/problem?id=699088">699088</a></span></div>'''
    assert recent.variant_members(html, VARIANT, "https://math-ege.sdamgia.ru") == [("699088", 4)]
    assert recent.variant_members(html.replace("2026", "2025"), VARIANT, "https://math-ege.sdamgia.ru") == []
