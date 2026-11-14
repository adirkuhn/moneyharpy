//go:build !race

package recognize

// raceSlowdown widens time budgets of tests run with the race detector.
const raceSlowdown = 1
