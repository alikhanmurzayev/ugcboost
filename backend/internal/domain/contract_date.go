package domain

import (
	"fmt"
	"time"
)

// IssuedDateMonths — родительный падеж русских названий месяцев (Decision #13
// intent-v2). Индексируется через time.Month-1.
var IssuedDateMonths = [...]string{
	"января", "февраля", "марта", "апреля", "мая", "июня",
	"июля", "августа", "сентября", "октября", "ноября", "декабря",
}

// FormatIssuedDate возвращает дату в формате «D» месяц YYYY г. в указанной
// локации. День — без leading zero, месяц — родительный падеж. Используется
// outbox-worker'ом для подстановки {{IssuedDate}} в шаблон договора.
func FormatIssuedDate(t time.Time, loc *time.Location) string {
	if loc == nil {
		loc = time.UTC
	}
	t = t.In(loc)
	return fmt.Sprintf("«%d» %s %d г.", t.Day(), IssuedDateMonths[t.Month()-1], t.Year())
}
