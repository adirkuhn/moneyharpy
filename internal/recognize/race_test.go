//go:build race

package recognize

// raceSlowdown widens time budgets of tests: the race detector makes code
// several times slower.
const raceSlowdown = 10
