// Expõe aos testes (pacote postgres_test) peças internas do pacote. Um
// arquivo _test.go no pacote "postgres" só entra na compilação dos testes,
// então nada disto vai para o binário.

package postgres

// MigrationsFS são as migrations embutidas, para o teste de subir e descer.
var MigrationsFS = migrationsFS
