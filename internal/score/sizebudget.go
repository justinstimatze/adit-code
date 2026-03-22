package score

// SizeGrade returns a grade based on line count thresholds from production experience.
func SizeGrade(lines int) string {
	switch {
	case lines < 500:
		return "A"
	case lines < 1500:
		return "B"
	case lines < 3000:
		return "C"
	case lines < 5000:
		return "D"
	default:
		return "F"
	}
}
