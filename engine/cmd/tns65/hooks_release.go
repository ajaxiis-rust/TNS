//go:build !e2e

package main

import "flag"

// В релизной сборке хуков нет: флаги --fake-* не регистрируются, ядро с ними
// не запускается (T-09-01, TestNoE2eHooksInRelease).
func addE2EBirthFlags(fs *flag.FlagSet) e2eBirthHooks { return nil }

func isE2ECoreFlag(arg string) bool { return false }

func runE2ECore(args []string) int { return 2 }
