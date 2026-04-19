package main

func (g *Game) AliveCounts() (mafia int, village int) {
	if g == nil {
		return 0, 0
	}

	for _, player := range g.Players {
		if !player.Alive {
			continue
		}

		switch player.Role {
		case RoleMafia:
			mafia++
		case RoleVillager, RoleDoctor, RoleDetective:
			village++
		}
	}

	return mafia, village
}

func (g *Game) CheckWin() (Winner, bool) {
	if g == nil {
		return WinnerNone, false
	}

	mafia, village := g.AliveCounts()

	if mafia == 0 {
		return WinnerVillage, true
	}

	if mafia >= village {
		return WinnerMafia, true
	}

	return WinnerNone, false
}
