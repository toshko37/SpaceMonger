package main

import (
	"os"
	"path/filepath"
	"testing"
)

func writeMounts(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "mounts")
	os.WriteFile(p, []byte(content), 0644)
	return p
}

func TestParseMounts_realFSOnly(t *testing.T) {
	p := writeMounts(t, `sysfs /sys sysfs rw,nosuid 0 0
proc /proc proc rw,nosuid 0 0
/dev/sda1 / ext4 rw,relatime 0 0
/dev/sda2 /home ext4 rw,relatime 0 0
tmpfs /tmp tmpfs rw,nosuid 0 0
/dev/sdb1 /data xfs rw,relatime 0 0
`)
	mounts, err := parseMountsFile(p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mounts) != 3 {
		t.Fatalf("expected 3 real mounts, got %d: %+v", len(mounts), mounts)
	}
	paths := map[string]bool{}
	for _, m := range mounts {
		paths[m.Path] = true
	}
	for _, want := range []string{"/", "/home", "/data"} {
		if !paths[want] {
			t.Errorf("missing mount path %s", want)
		}
	}
}

func TestParseMounts_filtersPseudo(t *testing.T) {
	p := writeMounts(t, `devpts /dev/pts devpts rw 0 0
tmpfs /run/user/1000 tmpfs rw 0 0
cgroup2 /sys/fs/cgroup cgroup2 rw 0 0
`)
	mounts, err := parseMountsFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(mounts) != 0 {
		t.Errorf("expected 0 real mounts, got %d", len(mounts))
	}
}

func TestParseMounts_dedupBindMountsKeepsShortest(t *testing.T) {
	p := writeMounts(t, `/dev/sda1 /var/lib/docker/overlay ext4 rw 0 0
/dev/sda1 / ext4 rw 0 0
`)
	mounts, _ := parseMountsFile(p)
	if len(mounts) != 1 || mounts[0].Path != "/" {
		t.Errorf("expected single mount at /, got %+v", mounts)
	}
}

func TestParseMounts_unescapesSpaces(t *testing.T) {
	p := writeMounts(t, `/dev/sdc1 /mnt/my\040disk ext4 rw 0 0
`)
	mounts, _ := parseMountsFile(p)
	if len(mounts) != 1 || mounts[0].Path != "/mnt/my disk" {
		t.Errorf("expected unescaped path, got %+v", mounts)
	}
}
