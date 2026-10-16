// Espera entre tentativas de um job da fila que falhou.

package domain

import "time"

// maxAttemptDelay é o teto da espera: 6 horas.
const maxAttemptDelay = 6 * time.Hour

// NextAttemptDelay implementa "backoff exponencial": cada falha dobra a
// espera (1, 2, 4, 8 min...). Assim não martelamos uma fonte que está fora
// do ar. O teto evita esperas absurdas.
//
// "time.Minute << attempt" desloca bits: é 1 minuto × 2^attempt.
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
