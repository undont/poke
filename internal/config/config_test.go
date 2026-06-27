package config

import "testing"

func TestSurfaceNormalises(t *testing.T) {
	cases := map[string]string{
		SurfaceTmux:    SurfaceTmux,
		SurfaceDesktop: SurfaceDesktop,
		SurfaceAuto:    SurfaceAuto,
		"":             SurfaceTmux, // empty falls back to the default
		"bogus":        SurfaceTmux, // a typo degrades rather than disabling cues
	}
	for in, want := range cases {
		if got := surface(in); got != want {
			t.Fatalf("surface(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestValidateSurface(t *testing.T) {
	for _, s := range []string{SurfaceTmux, SurfaceDesktop, SurfaceAuto} {
		if err := ValidateSurface(s); err != nil {
			t.Fatalf("ValidateSurface(%q) unexpected error: %v", s, err)
		}
	}
	for _, s := range []string{"", "bogus", "Tmux"} {
		if err := ValidateSurface(s); err == nil {
			t.Fatalf("ValidateSurface(%q) should reject", s)
		}
	}
}
