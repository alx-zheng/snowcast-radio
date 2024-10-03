package util

func Contains(s []string, e string) bool {
	for _, a := range s {
		if a == e {
			return true
		}
	}
	return false
}

func StringIndexOf(s []string, e string) int {
	for i, a := range s {
		if a == e {
			return i
		}
	}
	return -1
}

func RemoveIndex(s []string, index int) []string {
	return append(s[:index], s[index+1:]...)
}
