package domain

import "time"

const maxAttemptDelay = 6 * time.Hour

// NextAttemptDelay implementa "backoff exponencial": cada falha dobra a
// espera (1, 2, 4, 8 min...). Assim não martelamos uma fonte que está fora
// do ar. O teto evita esperas absurdas.
func NextAttemptDelay(attempt int) time.Duration {
	if attempt >= 9 { // 2^9 min > 6 h: evita overflow em números grandes
		return maxAttemptDelay
	}
	d := time.Minute << attempt
	if d > maxAttemptDelay {
		return maxAttemptDelay
	}
	return d
}
