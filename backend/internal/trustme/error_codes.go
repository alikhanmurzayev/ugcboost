package trustme

// FormatErrorText аугументирует raw errorText от TrustMe человекочитаемой
// расшифровкой кода. На вход — `wrapper.ErrorText` (обычно «1219», иногда
// «Some text or number error» если не код). Возвращает либо
// «1219 (Нет разрешения на автоподписание документа)» если код известен,
// либо raw как есть. Безопасна на пустой строке.
//
// Сами расшифровки живут в error_codes_data.gen.go и регенерятся скриптом
// gen.go из docs/external/trustme/blueprint.apib (см. make
// generate-trustme-codes). Орфография оригинала сохраняется 1-в-1, чтобы
// текст в логах совпадал byte-by-byte с тем, что оператор видит в blueprint.
func FormatErrorText(raw string) string {
	if raw == "" {
		return ""
	}
	if desc, ok := errorCodeDescriptions[raw]; ok {
		return raw + " (" + desc + ")"
	}
	return raw
}
