package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"termchat/shared"
)

func TestFetchOnlineRooms(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/discover" {
				t.Errorf("path = %q, want /discover", r.URL.Path)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[{"id":"ABCD","user_count":2,"host_nick":"alice"}]`))
		}))
		defer srv.Close()

		rooms, err := fetchOnlineRooms(srv.URL)
		if err != nil {
			t.Fatalf("fetchOnlineRooms: %v", err)
		}
		if len(rooms) != 1 || rooms[0].ID != "ABCD" {
			t.Fatalf("rooms = %+v, want one ABCD room", rooms)
		}
	})

	t.Run("wrong content-type", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/plain")
			_, _ = w.Write([]byte(`[]`))
		}))
		defer srv.Close()

		_, err := fetchOnlineRooms(srv.URL)
		if err == nil || !strings.Contains(err.Error(), "discovery") {
			t.Fatalf("err = %v, want discovery error", err)
		}
	})

	t.Run("bad status", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`[]`))
		}))
		defer srv.Close()

		_, err := fetchOnlineRooms(srv.URL)
		if err == nil || !strings.Contains(err.Error(), "discovery") {
			t.Fatalf("err = %v, want discovery error", err)
		}
	})

	t.Run("bad json", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`not json`))
		}))
		defer srv.Close()

		_, err := fetchOnlineRooms(srv.URL)
		if err == nil || !strings.Contains(err.Error(), "parsing") {
			t.Fatalf("err = %v, want parsing error", err)
		}
	})

	t.Run("conn error", func(t *testing.T) {
		_, err := fetchOnlineRooms("http://127.0.0.1:1")
		if err == nil || !strings.Contains(err.Error(), "Could not reach") {
			t.Fatalf("err = %v, want reach error", err)
		}
	})
}

func TestScanOnlineCmd(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"id":"ABCD","user_count":1,"host_nick":"alice"}]`))
	}))
	defer srv.Close()

	msg := scanOnlineCmd(srv.URL)()
	rooms, ok := msg.(onlineRoomsMsg)
	if !ok {
		t.Fatalf("cmd returned %T, want onlineRoomsMsg", msg)
	}
	if rooms.err != nil {
		t.Fatalf("rooms err = %v, want nil", rooms.err)
	}
	if len(rooms.rooms) != 1 || rooms.rooms[0].ID != "ABCD" {
		t.Fatalf("rooms = %+v, want one ABCD room", rooms.rooms)
	}

	errMsg := scanOnlineCmd("http://127.0.0.1:1")()
	failed, ok := errMsg.(onlineRoomsMsg)
	if !ok {
		t.Fatalf("cmd returned %T, want onlineRoomsMsg", errMsg)
	}
	if failed.err == nil {
		t.Error("conn error did not surface in onlineRoomsMsg")
	}
	var _ = shared.RoomInfo{}
}
