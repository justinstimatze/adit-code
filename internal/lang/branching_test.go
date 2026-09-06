package lang

import (
	"testing"
)

func TestComputeMaxBranching_ElseIfChains(t *testing.T) {
	tests := []struct {
		name     string
		frontend Frontend
		src      string
		want     int
	}{
		{
			name:     "go else-if chain",
			frontend: NewGoFrontend(),
			src: `package p
func f() {
	if a {
	} else if b {
	} else if c {
	} else if d {
	} else {
	}
}
`,
			want: 5,
		},
		{
			name:     "rust else-if chain",
			frontend: NewRustFrontend(),
			src: `fn f() {
	if a {
	} else if b {
	} else if c {
	} else if d {
	} else {
	}
}
`,
			want: 5,
		},
		{
			name:     "python elif chain unaffected",
			frontend: NewPythonFrontend(),
			src: `def f():
    if a:
        pass
    elif b:
        pass
    elif c:
        pass
    else:
        pass
`,
			want: 4,
		},
		{
			name:     "go plain if with no else",
			frontend: NewGoFrontend(),
			src: `package p
func f() {
	if a {
	}
}
`,
			want: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ext := tt.frontend.Extensions()[0]
			fa, err := tt.frontend.Analyze("test"+ext, []byte(tt.src))
			if err != nil {
				t.Fatalf("Analyze() error = %v", err)
			}
			if fa.MaxBranching != tt.want {
				t.Errorf("MaxBranching = %d, want %d", fa.MaxBranching, tt.want)
			}
		})
	}
}
