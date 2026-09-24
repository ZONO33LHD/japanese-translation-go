//go:build race

package lint

// race 検出器は実行を数倍遅くするので、計算量の退行検知の予算を広げる。
const raceEnabled = true
