package permission

import "testing"

func TestUnsafePersistentAllowRule(t *testing.T) {
	unsafe := []string{
		"Bash",
		"Bash(python -c:*)",
		"Bash(pwsh -EncodedCommand:*)",
		"Bash(git:*)",
		"Bash(rm -rf:*)",
		"Bash(npm run:*)",
		"Bash(node -e *)",
	}
	for _, rule := range unsafe {
		if !UnsafePersistentAllowRule(rule) {
			t.Errorf("UnsafePersistentAllowRule(%q) = false, want true", rule)
		}
	}

	safe := []string{
		"Bash(go test:*)",
		"Bash(git status:*)",
		"Bash(npm run test:*)",
		"Bash(python scripts/check.py:*)",
		"Bash=python -c print(1)",
		"Edit(src/**)",
	}
	for _, rule := range safe {
		if UnsafePersistentAllowRule(rule) {
			t.Errorf("UnsafePersistentAllowRule(%q) = true, want false", rule)
		}
	}
}

func TestPolicyIgnoresUnsafePersistentAllowRules(t *testing.T) {
	policy := New("ask", []string{"Bash(python -c:*)", "Bash(go test:*)"}, nil, nil)
	if got := policy.DecideSubject("bash", false, "python -c print(1)"); got != Ask {
		t.Fatalf("python decision = %s, want ask", got)
	}
	if got := policy.DecideSubject("bash", false, "go test ./..."); got != Allow {
		t.Fatalf("go test decision = %s, want allow", got)
	}
}

func TestRememberRuleFallsBackToExactUnsafeInterpreterCommand(t *testing.T) {
	got := RememberRuleForScope("bash", `python -c "print('ok')"`)
	if got != `Bash(python -c "print('ok')")` {
		t.Fatalf("RememberRuleForScope = %q, want exact command", got)
	}
	if UnsafePersistentAllowRule(got) {
		t.Fatalf("exact remembered command was classified unsafe: %q", got)
	}
}

func TestFilterUnsafePersistentAllowRulesPreservesOrder(t *testing.T) {
	safe, removed := FilterUnsafePersistentAllowRules([]string{
		"read_file",
		"Bash(python -c:*)",
		"Bash(go test:*)",
		"Bash",
	})
	if len(safe) != 2 || safe[0] != "read_file" || safe[1] != "Bash(go test:*)" {
		t.Fatalf("safe = %v", safe)
	}
	if len(removed) != 2 || removed[0] != "Bash(python -c:*)" || removed[1] != "Bash" {
		t.Fatalf("removed = %v", removed)
	}
}
