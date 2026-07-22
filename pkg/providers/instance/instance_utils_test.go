package instance

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	karpv1 "sigs.k8s.io/karpenter/pkg/apis/v1"

	"github.com/sergelogvinov/karpenter-provider-proxmox/pkg/apis/v1alpha1"
)

func TestNodeClaimBootMethod(t *testing.T) {
	tests := []struct {
		name        string
		annotations map[string]string
		wantPXE     bool
	}{
		{
			name:        "cloudInit default (empty annotation)",
			annotations: map[string]string{},
			wantPXE:     false,
		},
		{
			name:        "cloudInit explicit",
			annotations: map[string]string{v1alpha1.AnnotationProxmoxBootMethod: v1alpha1.BootMethodCloudInit},
			wantPXE:     false,
		},
		{
			name:        "pxe boot method",
			annotations: map[string]string{v1alpha1.AnnotationProxmoxBootMethod: v1alpha1.BootMethodPXE},
			wantPXE:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nc := &karpv1.NodeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: tt.annotations,
				},
			}
			isPXE := nc.Annotations[v1alpha1.AnnotationProxmoxBootMethod] == v1alpha1.BootMethodPXE
			assert.Equal(t, tt.wantPXE, isPXE)
		})
	}
}
