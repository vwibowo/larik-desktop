package desktop

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"larik/internal/integration"
)

func (a *App) customSkillSettings(c *ui.Context, k *palette, home string) {
	targets, listErr := integration.CustomTargets()
	a.settingCard(c, k, "Other agent harnesses", "Choose an existing skills folder. Larik copies its three skills there. Forget removes tracking but leaves the files.", "custom-skills", func() {
		if listErr != nil {
			ui.Text(c, listErr.Error()).FontSize(11.5).TextColor(ui.Hex("#c34b50")).Wrap()
		}
		if ui.Button(c, "Install in another agent…").Clicked() {
			path, err := a.pickSkillsFolder()
			if err == nil && path != "" {
				bin := filepath.Join(home, "Applications", "larik-desktop.app", "Contents", "MacOS", "larik-desktop")
				if _, err = os.Stat(bin); err == nil {
					err = integration.InstallAt(path, bin)
				} else {
					err = fmt.Errorf("install Larik Desktop first: %w", err)
				}
			}
			a.customSkillError("custom-skills", err)
		}
		for _, target := range targets {
			installed, statusErr := integration.StatusAt(target)
			field := "custom-skills:" + target
			status := "Installed"
			if statusErr != nil {
				status = "Needs attention"
			} else if !installed {
				status = "Missing"
			}
			ui.Column(c.Key(target)).FillWidth().Gap(5).Children(func() {
				ui.Text(c, target).FontSize(11.5).TextColor(k.text).SingleLine().Ellipsis("…")
				ui.Text(c, status).FontSize(11).TextColor(k.muted)
				if statusErr != nil {
					ui.Text(c, statusErr.Error()).FontSize(11).TextColor(ui.Hex("#c34b50")).Wrap()
				}
				ui.Row(c).Gap(6).Children(func() {
					if ui.Button(c, "Update").Clicked() {
						bin := filepath.Join(home, "Applications", "larik-desktop.app", "Contents", "MacOS", "larik-desktop")
						_, err := os.Stat(bin)
						if err == nil {
							err = integration.InstallAt(target, bin)
						} else {
							err = fmt.Errorf("install Larik Desktop first: %w", err)
						}
						a.customSkillError(field, err)
					}
					if ui.Button(c, "Remove").Clicked() {
						a.customSkillError(field, integration.RemoveAt(target))
					}
					if ui.Button(c, "Forget").Clicked() {
						a.customSkillError(field, integration.ForgetAt(target))
					}
				})
				if a.settingsField == field && a.settingsErr != "" {
					ui.Text(c, a.settingsErr).FontSize(11).TextColor(ui.Hex("#c34b50")).Wrap()
				}
			})
		}
	})
}

func (a *App) pickSkillsFolder() (string, error) {
	if a.chooseSkillsFolder != nil {
		return a.chooseSkillsFolder()
	}
	paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: a.settingsWin, Title: "Choose the agent's skills folder", Directory: true})
	if err != nil || len(paths) == 0 {
		return "", err
	}
	return paths[0], nil
}

func (a *App) customSkillError(field string, err error) {
	if err != nil {
		a.settingsField, a.settingsErr = field, err.Error()
	} else {
		a.settingsField, a.settingsErr = "", ""
	}
}
