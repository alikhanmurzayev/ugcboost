package domain

import "strings"

// NormalizePhoneE164 приводит телефон к E.164 для KZ. БД хранит phone как
// ввёл пользователь; нормализация нужна только в TrustMe SendToSign.Requisites.
//
// Алгоритм: оставляем '+' и цифры, убираем всё прочее. Если не начинается с
// '+', а первый значащий символ '8' или '7' — заменяем на '+7'. Если начинается
// с '+8' — нормализуем в '+7'.
//
// Возвращает входную строку без изменений, если не получилось привести к
// E.164 (нет цифр, явно нерегиональный формат) — caller сам решает, что слать.
func NormalizePhoneE164(raw string) string {
	if raw == "" {
		return ""
	}
	var sb strings.Builder
	plus := false
	for i, r := range raw {
		switch {
		case r == '+' && i == 0:
			sb.WriteRune(r)
			plus = true
		case r >= '0' && r <= '9':
			sb.WriteRune(r)
		}
	}
	digits := sb.String()
	if digits == "" {
		return raw
	}
	if plus {
		if strings.HasPrefix(digits, "+8") && len(digits) == 12 {
			return "+7" + digits[2:]
		}
		return digits
	}
	if (strings.HasPrefix(digits, "8") || strings.HasPrefix(digits, "7")) && len(digits) == 11 {
		return "+7" + digits[1:]
	}
	return digits
}
