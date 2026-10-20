package ui

import "el-book/sdk/ru"

// Plural выбирает форму слова для числа n — ru.Plural; оставлено для старого кода.
func Plural(n int, one, few, many string) string { return ru.Plural(n, one, few, many) }

// Count — число со словом в нужной форме — ru.Count.
func Count(n int, one, few, many string) string { return ru.Count(n, one, few, many) }
