//go:build !darwin

package wtl

import "context"

func moveActiveChromeTabToGroup(context.Context, string) error {
	return nil
}
