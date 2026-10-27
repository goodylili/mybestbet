package betano_test

import (
	"errors"
	"testing"

	"mybestbet/pkg/canon"
	"mybestbet/pkg/sources"
	"mybestbet/pkg/sources/betano"
)

func TestBookIsBlocked(t *testing.T) {
	t.Parallel()
	booker, ok := betano.New(sources.Deps{}).(sources.Booker)
	if !ok {
		t.Fatal("betano adapter does not implement sources.Booker")
	}
	_, err := booker.Book(t.Context(), []sources.BookLeg{{SourceEventID: "1", Code: canon.Code1X2, Side: string(canon.SideHome)}})
	if !errors.Is(err, sources.ErrBookingBlocked) {
		t.Fatalf("error = %v, want %v", err, sources.ErrBookingBlocked)
	}
}
