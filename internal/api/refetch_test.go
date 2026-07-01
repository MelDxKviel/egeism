package api

import "testing"

// isTheoryBloat is the predicate that flags РЕШУ theory-card bloat (the русский
// pbodies[0] bug) for re-fetch. It must catch a dumped theory card but never a
// real condition — the fetcher fix stops new bloat, this heals what's stored.
func TestIsTheoryBloat(t *testing.T) {
	bloated := []string{
		"Задание 1. Логико-смысловые отношения. Что проверяется и что нужно знать? Элементы содержания по «Кодификатору».",
		"Необходимые материалы к заданию: Справочник РЕШУ ЕГЭ. Файлы для скачивания: Список частей речи (PDF).",
		"АЛГОРИТМ. Шаг 1. Файлы для скачивания.",
	}
	for _, s := range bloated {
		if !isTheoryBloat(s) {
			t.Errorf("expected bloat, got clean for: %.40q", s)
		}
	}

	clean := []string{
		"Самостоятельно подберите разделительный союз, который должен стоять на месте пропуска в первом (1) предложении текста. Запишите этот союз.",
		"В треугольнике ABC угол C равен 90°, AC = 4,8. Найдите AB.",
		"Ниже приведён ряд политических партий. Найдите два термина, «выпадающих» из общего ряда.",
		"",
	}
	for _, s := range clean {
		if isTheoryBloat(s) {
			t.Errorf("expected clean, got bloat for: %.40q", s)
		}
	}
}
