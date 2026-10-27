package main

import "fmt"

// memStat prints what the loaded bundles weigh, by kind.
func (x *X) memStat() {
	var files, res int
	for _, f := range x.Env.Files {
		for _, o := range f.Objects {
			files += int(o.Size)
		}
	}
	big := map[string]int{}
	for n, b := range x.Env.Res {
		res += len(b)
		if len(b) > 50<<20 {
			big[n] = len(b)
		}
	}
	fmt.Printf("    objects %d MB, resources %d MB, big resources %v\n", files>>20, res>>20, big)
}
