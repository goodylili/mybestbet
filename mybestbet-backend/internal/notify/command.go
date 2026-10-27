package notify

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"mybestbet/pkg/canon"
)

const (
	maxMuteDuration = 30 * 24 * time.Hour
	maxPercent      = 100
)

const helpText = `Commands:
/arbs 1.0 – arbs at 1.0% margin or more
/ev 5 – +EV at 5% or more
/league EPL LALIGA NBA – limit to these leagues (no codes = all)
/sport football basketball – limit to these sports (no sports = all): football basketball tennis american-football esports
/props on|off – include player prop signals in every sport
/move 10 – implied probability moves of 10% or more (off to disable)
/mute 2h – pause alerts (off to resume)`

type command struct {
	name string
	args []string
}

type usageError struct{ hint string }

func (e usageError) Error() string { return e.hint }

func parseCommand(text string) (command, bool) {
	fields := strings.Fields(text)
	if len(fields) == 0 || !strings.HasPrefix(fields[0], "/") {
		return command{}, false
	}
	name, _, _ := strings.Cut(strings.TrimPrefix(fields[0], "/"), "@")
	return command{name: strings.ToLower(name), args: fields[1:]}, true
}

type commandEnv struct {
	now          time.Time
	knownLeagues []string
}

func applyCommand(sub Subscription, cmd command, env commandEnv) (Subscription, string, error) {
	switch cmd.name {
	case "start", "help":
		return sub, "Subscribed. " + describe(sub) + "\n\n" + helpText, nil
	case "arbs":
		return withPercent(sub, cmd, "/arbs 1.0", func(s *Subscription, v float64) { s.MinArbPercent = v })
	case "ev":
		return withPercent(sub, cmd, "/ev 5", func(s *Subscription, v float64) { s.MinEVPercent = v })
	case "league", "leagues":
		return withLeagues(sub, cmd.args, env.knownLeagues)
	case "sport", "sports":
		return withSports(sub, cmd.args)
	case "props":
		return withProps(sub, cmd.args)
	case "move":
		return withMove(sub, cmd.args)
	case "mute":
		return withMute(sub, cmd.args, env.now)
	}
	return sub, "", usageError{hint: helpText}
}

func withPercent(sub Subscription, cmd command, example string, set func(*Subscription, float64)) (Subscription, string, error) {
	value, err := singlePercent(cmd.args)
	if err != nil {
		return sub, "", usageError{hint: "Usage: " + example}
	}
	set(&sub, value)
	return sub, describe(sub), nil
}

func singlePercent(args []string) (float64, error) {
	if len(args) != 1 {
		return 0, fmt.Errorf("want one value")
	}
	value, err := strconv.ParseFloat(strings.TrimSuffix(args[0], "%"), 64)
	if err != nil || value < 0 || value > maxPercent {
		return 0, fmt.Errorf("want a percentage between 0 and 100")
	}
	return value, nil
}

func withLeagues(sub Subscription, args, known []string) (Subscription, string, error) {
	leagues, err := distinctKnown(args, known, strings.ToUpper, "league")
	if err != nil {
		return sub, "", err
	}
	sub.Leagues = leagues
	return sub, describe(sub), nil
}

var knownSports = func() []string {
	names := make([]string, len(canon.Sports))
	for i, sport := range canon.Sports {
		names[i] = string(sport)
	}
	return names
}()

func withSports(sub Subscription, args []string) (Subscription, string, error) {
	sports, err := distinctKnown(args, knownSports, strings.ToLower, "sport")
	if err != nil {
		return sub, "", err
	}
	sub.Sports = sports
	return sub, describe(sub), nil
}

func distinctKnown(args, known []string, normalize func(string) string, kind string) ([]string, error) {
	picked := []string{}
	for _, arg := range args {
		value := normalize(arg)
		if !slices.Contains(known, value) {
			return nil, usageError{hint: "Unknown " + kind + " " + arg + ". Known: " + strings.Join(known, " ")}
		}
		if !slices.Contains(picked, value) {
			picked = append(picked, value)
		}
	}
	return picked, nil
}

func withProps(sub Subscription, args []string) (Subscription, string, error) {
	enabled, ok := onOff(args)
	if !ok {
		return sub, "", usageError{hint: "Usage: /props on|off"}
	}
	sub.Props = enabled
	return sub, describe(sub), nil
}

func withMove(sub Subscription, args []string) (Subscription, string, error) {
	if enabled, ok := onOff(args); ok && !enabled {
		sub.MovePercent = nil
		return sub, describe(sub), nil
	}
	value, err := singlePercent(args)
	if err != nil || value == 0 {
		return sub, "", usageError{hint: "Usage: /move 10 or /move off"}
	}
	sub.MovePercent = &value
	return sub, describe(sub), nil
}

func withMute(sub Subscription, args []string, now time.Time) (Subscription, string, error) {
	if enabled, ok := onOff(args); ok && !enabled {
		sub.MutedUntil = nil
		return sub, describe(sub), nil
	}
	if len(args) != 1 {
		return sub, "", usageError{hint: "Usage: /mute 2h or /mute off"}
	}
	duration, err := time.ParseDuration(args[0])
	if err != nil || duration <= 0 || duration > maxMuteDuration {
		return sub, "", usageError{hint: "Usage: /mute 2h (up to 720h) or /mute off"}
	}
	until := now.Add(duration).UTC()
	sub.MutedUntil = &until
	return sub, describe(sub), nil
}

func onOff(args []string) (enabled, ok bool) {
	if len(args) != 1 {
		return false, false
	}
	switch strings.ToLower(args[0]) {
	case "on":
		return true, true
	case "off":
		return false, true
	}
	return false, false
}

func describe(sub Subscription) string {
	leagues := "all leagues"
	if len(sub.Leagues) > 0 {
		leagues = strings.Join(sub.Leagues, " ")
	}
	sports := "all sports"
	if len(sub.Sports) > 0 {
		sports = strings.Join(sub.Sports, " ")
	}
	move := "off"
	if sub.MovePercent != nil {
		move = strconv.FormatFloat(*sub.MovePercent, 'f', -1, 64) + "%"
	}
	muted := "no"
	if sub.MutedUntil != nil {
		muted = "until " + sub.MutedUntil.Format(time.RFC3339)
	}
	return fmt.Sprintf("Arbs ≥ %s%%, EV ≥ %s%%, %s, %s, props %s, moves %s, muted %s.",
		strconv.FormatFloat(sub.MinArbPercent, 'f', -1, 64), strconv.FormatFloat(sub.MinEVPercent, 'f', -1, 64),
		leagues, sports, onOffLabel(sub.Props), move, muted)
}

func onOffLabel(enabled bool) string {
	if enabled {
		return "on"
	}
	return "off"
}
