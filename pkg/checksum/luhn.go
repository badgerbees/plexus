package checksum

func LuhnGenerate(prefix string, totalLen int) string {
	digits := make([]byte, totalLen)
	for i := range digits {
		if i < len(prefix) {
			digits[i] = prefix[i] - '0'
		} else if i < totalLen-1 {
			digits[i] = byte((i * 7 + 3) % 10)
		}
	}

	sum := 0
	alt := true
	for i := totalLen - 2; i >= 0; i-- {
		n := int(digits[i])
		if alt {
			n *= 2
			if n > 9 {
				n -= 9
			}
		}
		sum += n
		alt = !alt
	}
	check := (10 - (sum % 10)) % 10
	digits[totalLen-1] = byte(check)

	out := make([]byte, totalLen)
	for i, d := range digits {
		out[i] = d + '0'
	}
	return string(out)
}

func LuhnValid(number string) bool {
	sum := 0
	alt := false
	for i := len(number) - 1; i >= 0; i-- {
		n := int(number[i] - '0')
		if n < 0 || n > 9 {
			return false
		}
		if alt {
			n *= 2
			if n > 9 {
				n -= 9
			}
		}
		sum += n
		alt = !alt
	}
	return sum%10 == 0
}
