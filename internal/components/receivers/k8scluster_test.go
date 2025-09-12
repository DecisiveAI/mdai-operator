// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package receivers

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	rbacv1 "k8s.io/api/rbac/v1"
)

func Test_generatek8sclusterRbacRules(t *testing.T) {
	logger := zap.NewNop()
	tests := []struct {
		name    string
		cfg     k8sclusterConfig
		want    []rbacv1.PolicyRule
		wantErr bool
	}{
		{
			name: "default configuration",
			cfg:  k8sclusterConfig{},
			want: []rbacv1.PolicyRule{
				{
					APIGroups: []string{""},
					Resources: []string{
						"events",
						"namespaces",
						"namespaces/status",
						"nodes",
						"nodes/spec",
						"pods",
						"pods/status",
						"replicationcontrollers",
						"replicationcontrollers/status",
						"resourcequotas",
						"services",
					},
					Verbs: []string{"get", "list", "watch"},
				},
				{
					APIGroups: []string{"apps"},
					Resources: []string{
						"daemonsets",
						"deployments",
						"replicasets",
						"statefulsets",
					},
					Verbs: []string{"get", "list", "watch"},
				},
				{
					APIGroups: []string{"extensions"},
					Resources: []string{
						"daemonsets",
						"deployments",
						"replicasets",
					},
					Verbs: []string{"get", "list", "watch"},
				},
				{
					APIGroups: []string{"batch"},
					Resources: []string{
						"jobs",
						"cronjobs",
					},
					Verbs: []string{"get", "list", "watch"},
				},
				{
					APIGroups: []string{"autoscaling"},
					Resources: []string{"horizontalpodautoscalers"},
					Verbs:     []string{"get", "list", "watch"},
				},
			},
			wantErr: false,
		},
		{
			name: "openshift configuration",
			cfg: k8sclusterConfig{
				Distribution: "openshift",
			},
			want: []rbacv1.PolicyRule{
				{
					APIGroups: []string{""},
					Resources: []string{
						"events",
						"namespaces",
						"namespaces/status",
						"nodes",
						"nodes/spec",
						"pods",
						"pods/status",
						"replicationcontrollers",
						"replicationcontrollers/status",
						"resourcequotas",
						"services",
					},
					Verbs: []string{"get", "list", "watch"},
				},
				{
					APIGroups: []string{"apps"},
					Resources: []string{
						"daemonsets",
						"deployments",
						"replicasets",
						"statefulsets",
					},
					Verbs: []string{"get", "list", "watch"},
				},
				{
					APIGroups: []string{"extensions"},
					Resources: []string{
						"daemonsets",
						"deployments",
						"replicasets",
					},
					Verbs: []string{"get", "list", "watch"},
				},
				{
					APIGroups: []string{"batch"},
					Resources: []string{
						"jobs",
						"cronjobs",
					},
					Verbs: []string{"get", "list", "watch"},
				},
				{
					APIGroups: []string{"autoscaling"},
					Resources: []string{"horizontalpodautoscalers"},
					Verbs:     []string{"get", "list", "watch"},
				},
				{
					APIGroups: []string{"quota.openshift.io"},
					Resources: []string{"clusterresourcequotas"},
					Verbs:     []string{"get", "list", "watch"},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := generatek8sclusterRbacRules(logger, tt.cfg)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
