// access-admin is an operator-only bootstrap tool; no first-login privilege escalation.
package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
	"glorynavy.local/seat/internal/modules/access"
	"glorynavy.local/seat/internal/modules/identity"
	"os"
	"time"
)

func main() {
	character := flag.Int64("character", 0, "An existing, active EVE character ID")
	revoke := flag.Bool("revoke", false, "Remove administrator access")
	flag.Parse()
	if *character <= 0 {
		fmt.Fprintln(os.Stderr, "--character must identify an existing signed-in character")
		os.Exit(1)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		fail()
	}
	defer pool.Close()
	user, err := identity.New(pool).UserForCharacter(ctx, *character)
	if err != nil {
		fail()
	}
	if err = access.New(pool, nil, nil).SetAdministrator(ctx, user, !*revoke); err != nil {
		fail()
	}
	fmt.Println("Administrator access updated for user", user)
}
func fail() {
	fmt.Fprintln(os.Stderr, "Administrator update failed; check DATABASE_URL and the active character ID")
	os.Exit(1)
}
