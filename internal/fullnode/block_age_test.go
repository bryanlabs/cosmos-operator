package fullnode

import (
	"testing"
	"time"

	cosmosv1 "github.com/bryanlabs/cosmos-operator/api/v1"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestMaxBlockAge(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		value string
		set   bool
		want  time.Duration
	}{
		{set: false, want: 0},
		{value: "60s", set: true, want: time.Minute},
		{value: "2m", set: true, want: 2 * time.Minute},
		{value: "0s", set: true, want: 0},
		{value: "-5s", set: true, want: 0},
		{value: "soon", set: true, want: 0},
	} {
		crd := defaultCRD()
		if tt.set {
			crd.Annotations = map[string]string{maxBlockAgeAnnotation: tt.value}
		}
		require.Equal(t, tt.want, maxBlockAge(&crd), tt.value)
	}
}

func TestHealthCheckCmd(t *testing.T) {
	t.Parallel()

	crd := defaultCRD()
	require.Equal(t, []string{"/manager", "healthcheck", "--rpc-host", "http://localhost:26657"}, healthCheckCmd(&crd))

	crd.Annotations = map[string]string{maxBlockAgeAnnotation: "60s"}
	require.Equal(t, []string{"/manager", "healthcheck", "--rpc-host", "http://localhost:26657", "--max-block-age", "1m0s"}, healthCheckCmd(&crd))
}

func TestHaltedForUpgrade(t *testing.T) {
	t.Parallel()

	newCRD := func(height uint64) *cosmosv1.CosmosFullNode {
		crd := defaultCRD()
		crd.Name = "hub"
		crd.Spec.ChainSpec.Versions = []cosmosv1.ChainVersion{
			{Image: "gaia:v27"},
			{UpgradeHeight: 100, Image: "gaia:v28"},
			{UpgradeHeight: 200, Image: "gaia:v29"},
		}
		crd.Status.Height = map[string]uint64{"hub-0": height}
		return &crd
	}
	pod := func(image string) *corev1.Pod {
		return &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "hub-0"},
			Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "node", Image: image}}},
		}
	}

	t.Run("old image at upgrade height", func(t *testing.T) {
		require.True(t, haltedForUpgrade(newCRD(200), pod("gaia:v28")))
	})

	t.Run("image two versions behind", func(t *testing.T) {
		require.True(t, haltedForUpgrade(newCRD(200), pod("gaia:v27")))
	})

	t.Run("running the image its height calls for", func(t *testing.T) {
		require.False(t, haltedForUpgrade(newCRD(150), pod("gaia:v28")))
	})

	t.Run("below the next upgrade height", func(t *testing.T) {
		require.False(t, haltedForUpgrade(newCRD(199), pod("gaia:v28")))
	})

	t.Run("image replaced in place for the current range", func(t *testing.T) {
		// A patch release swapped into the active entry is a normal rolling update, not a halt.
		require.False(t, haltedForUpgrade(newCRD(150), pod("gaia:v28-old-patch")))
	})

	t.Run("no recorded height", func(t *testing.T) {
		crd := newCRD(200)
		crd.Status.Height = nil
		require.False(t, haltedForUpgrade(crd, pod("gaia:v28")))
	})

	t.Run("instance image override", func(t *testing.T) {
		crd := newCRD(200)
		crd.Spec.InstanceOverrides = map[string]cosmosv1.InstanceOverridesSpec{"hub-0": {Image: "gaia:custom"}}
		require.False(t, haltedForUpgrade(crd, pod("gaia:v28")))
	})

	t.Run("no versions", func(t *testing.T) {
		crd := defaultCRD()
		crd.Status.Height = map[string]uint64{"hub-0": 200}
		require.False(t, haltedForUpgrade(&crd, pod("gaia:v28")))
	})
}
