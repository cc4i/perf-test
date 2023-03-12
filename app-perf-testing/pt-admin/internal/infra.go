package internal

import (
	"context"
	"fmt"
	"time"

	compute "cloud.google.com/go/compute/apiv1"
	computepb "cloud.google.com/go/compute/apiv1/computepb"
	container "cloud.google.com/go/container/apiv1"
	containerpb "cloud.google.com/go/container/apiv1/containerpb"
	admin "cloud.google.com/go/iam/admin/apiv1"
	adminpb "cloud.google.com/go/iam/admin/apiv1/adminpb"
	"google.golang.org/api/cloudresourcemanager/v1"
)

// TInfra is the struct for testing infrastructure, which is to be provisioned
type TInfra struct {
	// service account for all related operations
	ServiceAccount string
	// Key for the service account
	Key string
	// All provisoned GKE clusters: (region, cluster)
	Clusters map[string]GKECluster
}

// GKECluster is the struct for GKE cluster
type GKECluster struct {
	//Network
	Network string
	//Subnetwork
	Subnet string
	// Cluster name
	Name string
	// Cluster region
	Region string
	// Certificate of the cluster
	CA string
	// External IP of the cluster
	Endpoint string
}

type OperationalStuff interface {
	// Create a VPC network
	CreateVPCNetwork(ctx context.Context, projectId string, name string, mtu int32) (*compute.Operation, error)
	// Create a service account
	CreateServiceAccount(ctx context.Context, projectId string, accountId string) (*adminpb.ServiceAccount, error)
	// Create a service account key
	CreateServiceAccountKey(ctx context.Context, projectId string, accountId string) (*adminpb.ServiceAccountKey, error)
	// Set IAM policies to a service account
	SetIamPolicies2SA(ctx context.Context, projectId string, saName string) error
	// Create a GKE Autopilot cluster
	CreateAutopilotCluster(ctx context.Context, projectId string, cluster string, region string, network string, subnet string) (*containerpb.Operation, error)
	// Create a zonal GKE cluster
	CreateZonalCluster(ctx context.Context, projectId string, cluster string, zone string, network string, subnet string, saName string) (*containerpb.Operation, error)
}

func (ti *TInfra) CreateVPCNetwork(ctx context.Context, projectId string, name string, mtu int32) (*compute.Operation, error) {

	// https://cloud.google.com/go/docs/reference/cloud.google.com/go/networkmanagement/latest/apiv1
	c, err := compute.NewNetworksRESTClient(ctx)
	if err != nil {
		return &compute.Operation{}, err
	}
	defer c.Close()
	auto := true
	mode := "REGIONAL"

	return c.Insert(ctx, &computepb.InsertNetworkRequest{
		Project: projectId,
		NetworkResource: &computepb.Network{
			Name:                  &name,
			AutoCreateSubnetworks: &auto,
			Mtu:                   &mtu,
			RoutingConfig: &computepb.NetworkRoutingConfig{
				RoutingMode: &mode,
			},
		},
	})

}

func (ti *TInfra) CreateAutopilotCluster(ctx context.Context, projectId string, cluster string, region string, network string, subnet string) (*containerpb.Operation, error) {
	// https://cloud.google.com/go/docs/reference/cloud.google.com/go/container/apiv1

	c, err := container.NewClusterManagerClient(ctx)
	if err != nil {
		return &containerpb.Operation{}, err
	}
	defer c.Close()

	req := &containerpb.CreateClusterRequest{
		Parent: fmt.Sprintf("projects/%s/locations/%s", projectId, region),
		Cluster: &containerpb.Cluster{
			Name:       cluster,
			Network:    network,
			Subnetwork: subnet,
			Autopilot: &containerpb.Autopilot{
				Enabled: true,
			},
			ReleaseChannel: &containerpb.ReleaseChannel{
				Channel: containerpb.ReleaseChannel_RAPID,
			},
		},
	}
	return c.CreateCluster(ctx, req)

}

func (ti *TInfra) CreateServiceAccount(ctx context.Context, projectId string, accountId string) (*adminpb.ServiceAccount, error) {
	c, err := admin.NewIamClient(ctx)
	if err != nil {
		return &adminpb.ServiceAccount{}, err
	}

	req := &adminpb.CreateServiceAccountRequest{
		Name:      fmt.Sprintf("projects/%s", projectId),
		AccountId: accountId,
	}

	return c.CreateServiceAccount(ctx, req)

}

func (ti *TInfra) CreateServiceAccountKey(ctx context.Context, projectId string, accountId string) (*adminpb.ServiceAccountKey, error) {
	c, err := admin.NewIamClient(ctx)
	if err != nil {
		return &adminpb.ServiceAccountKey{}, err
	}

	req := &adminpb.CreateServiceAccountKeyRequest{
		Name: fmt.Sprintf("projects/%s/serviceAccounts/%s@%s.iam.gserviceaccount.com", projectId, accountId, projectId),
	}

	return c.CreateServiceAccountKey(ctx, req)

}

// Grant following roles to a specific service account:
//
//	roles/iam.workloadIdentityUser
//	roles/container.clusterAdmin
//	roles/artifactregistry.reader
//	roles/iam.serviceAccountUser
//	roles/storage.objectCreator
//	roles/compute.networkAdmin
//	roles/logging.admin
//	roles/monitoring.admin
//	roles/resourcemanager.projectIamAdmin
func (ti *TInfra) SetIamPolicies2SA(ctx context.Context, projectId string, saName string) error {

	crm, err := cloudresourcemanager.NewService(ctx)
	if err != nil {
		return err
	}

	roles := []string{
		"roles/iam.workloadIdentityUser",
		"roles/container.clusterAdmin",
		"roles/artifactregistry.reader",
		"roles/iam.serviceAccountUser",
		"roles/storage.objectCreator",
		"roles/compute.networkAdmin",
		"roles/logging.admin",
		"roles/monitoring.admin",
		"roles/resourcemanager.projectIamAdmin",
	}
	member := fmt.Sprintf("serviceAccount:%s@%s.iam.gserviceaccount.com", saName, projectId)
	for _, role := range roles {
		if policy, err := getPolicy(crm, projectId); err != nil {
			return err
		} else {
			// Find the policy binding for role. Only one binding can have the role.
			var binding *cloudresourcemanager.Binding
			for _, b := range policy.Bindings {
				if b.Role == role {
					binding = b
					break
				}
			}

			if binding != nil {
				// If the binding exists, adds the member to the binding
				binding.Members = append(binding.Members, member)
			} else {
				// If the binding does not exist, adds a new binding to the policy
				binding = &cloudresourcemanager.Binding{
					Role:    role,
					Members: []string{member},
				}
				policy.Bindings = append(policy.Bindings, binding)
			}

			if err := setPolicy(crm, projectId, policy); err != nil {
				return err
			}
		}

	}

	return nil
}

func getPolicy(crmService *cloudresourcemanager.Service, projectID string) (*cloudresourcemanager.Policy, error) {

	ctx := context.Background()

	ctx, cancel := context.WithTimeout(ctx, time.Second*10)
	defer cancel()
	request := new(cloudresourcemanager.GetIamPolicyRequest)
	return crmService.Projects.GetIamPolicy(projectID, request).Do()

}

func setPolicy(crmService *cloudresourcemanager.Service, projectID string, policy *cloudresourcemanager.Policy) error {

	ctx := context.Background()

	ctx, cancel := context.WithTimeout(ctx, time.Second*10)
	defer cancel()
	request := new(cloudresourcemanager.SetIamPolicyRequest)
	request.Policy = policy
	_, err := crmService.Projects.SetIamPolicy(projectID, request).Do()
	return err
}

func (ti *TInfra) GetEndpoint(ctx context.Context, projectId string, cluster string, region string) (string, error) {
	c, err := container.NewClusterManagerClient(ctx)
	if err != nil {
		return "", err
	}
	defer c.Close()

	req := &containerpb.GetClusterRequest{
		Name: fmt.Sprintf("projects/%s/locations/%s/clusters/%s", projectId, region, cluster),
	}
	resp, err := c.GetCluster(ctx, req)
	if err != nil {
		return "", err
	}

	return resp.Endpoint, nil
}

func (ti *TInfra) GetClusterCaCertificate(ctx context.Context, projectId string, cluster string, region string) (string, error) {
	c, err := container.NewClusterManagerClient(ctx)
	if err != nil {
		return "", err
	}
	defer c.Close()

	req := &containerpb.GetClusterRequest{
		Name: fmt.Sprintf("projects/%s/locations/%s/clusters/%s", projectId, region, cluster),
	}
	resp, err := c.GetCluster(ctx, req)
	if err != nil {
		return "", err
	}
	fmt.Println(resp.MasterAuth.ClusterCaCertificate)
	return resp.MasterAuth.ClusterCaCertificate, nil
}

func (ti *TInfra) CreateZonalCluster(ctx context.Context, projectId string, cluster string, zone string, network string, subnet string, saName string) (*containerpb.Operation, error) {
	// https://cloud.google.com/go/docs/reference/cloud.google.com/go/container/apiv1

	c, err := container.NewClusterManagerClient(ctx)
	if err != nil {
		return &containerpb.Operation{}, err
	}
	defer c.Close()

	req := &containerpb.CreateClusterRequest{
		Parent: fmt.Sprintf("projects/%s/locations/%s", projectId, zone),
		Cluster: &containerpb.Cluster{
			Name:       cluster,
			Network:    network,
			Subnetwork: subnet,
			Locations:  []string{zone},
			NodeConfig: &containerpb.NodeConfig{
				GcfsConfig: &containerpb.GcfsConfig{
					Enabled: true,
				},
			},
			NodePools: []*containerpb.NodePool{
				{
					Name: "default-pool",
					Config: &containerpb.NodeConfig{
						MachineType:    "e2-standard-2",
						DiskSizeGb:     40,
						OauthScopes:    []string{"https://www.googleapis.com/auth/cloud-platform"},
						ServiceAccount: fmt.Sprintf("%s@%s.iam.gserviceaccount.com", saName, projectId),
					},
					Locations:        []string{zone},
					InitialNodeCount: 1,
				},
			},
			ReleaseChannel: &containerpb.ReleaseChannel{
				Channel: containerpb.ReleaseChannel_RAPID,
			},
			AddonsConfig: &containerpb.AddonsConfig{
				GcpFilestoreCsiDriverConfig: &containerpb.GcpFilestoreCsiDriverConfig{
					Enabled: true,
				},
			},
			MonitoringConfig: &containerpb.MonitoringConfig{
				ManagedPrometheusConfig: &containerpb.ManagedPrometheusConfig{
					Enabled: true,
				},
			},
		},
	}
	return c.CreateCluster(ctx, req)

}
