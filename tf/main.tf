
provider "google" {
  project_id = var.project_id
  region  = var.region

}

// Enable APIs
resource "google_project_service" "project" {
  for_each = toset([
    "cloudresourcemanager.googleapis.com",
    "compute.googleapis.com",
    "container.googleapis.com",
    "containerregistry.googleapis.com",
    "iam.googleapis.com",
    "logging.googleapis.com",
    "monitoring.googleapis.com",
    "storage-api.googleapis.com",
    "storage-component.googleapis.com",
    "workflows.googleapis.com",
    "run.googleapis.com",
    "monitoring.googleapis.com",
    "build.googleapis.com",
    "clouddeploy.googleapis.com",
  ])

  service = each.key
}

// Provision a service account
resource "google_service_account" "service_account" {
  account_id   = var.service_account_name
  display_name = var.service_account_name
}

// Grant service account with GKE admin role
resource "google_project_iam_member" "service_account_gke_admin" {
  role   = "roles/container.admin"
  member = "serviceAccount:${google_service_account.service_account.email}"
}

// Grant service account with multiple roles: Storage Admin, Secret Manager Admin, Cloud Run Admin, Cloud Functions Admin, Cloud Scheduler Admin, Cloud Tasks Admin, Cloud Trace Admin, Cloud Workflows Admin
resource "google_project_iam_member" "service_account_storage_admin" {
  role   = "roles/storage.admin"
  member = "serviceAccount:${google_service_account.service_account.email}"
}


// Provision bucket to store test results
resource "google_storage_bucket" "bucket" {
  name          = var.bucket_name
  location      = var.region
  force_destroy = true
}

// Provision a artifact registry for Docker images
resource "google_artifact_registry_repository" "repository" {
  provider = google-beta
  location = var.region
  name     = var.artifact_registry_name
  format   = "DOCKER"
}

// Provision a firestore database with a collection
resource "google_firestore_database" "database" {
  project = var.project
  name    = var.firestore_database_name
}



// Provision a Cloud Run service with specified service account
resource "google_cloud_run_service" "example_service" {
  name     = "example-service"
  location = var.location

  template {
    spec {
      service_account_name = google_service_account.service_account.email
      containers {
        image = "gcr.io/cloudrun/hello"
      }
    }
  }
}


// Provision a workflow to run provision infrastructure and run tests
resource "google_workflows_workflow" "example_workflow" {
  name = "example-workflow"
  source_contents = <<EOF
  - step1:
      call: http.get
      args:
        url: "https://www.example.com"
      result: http_response
  - step2:
      call: googleapis.cloudfunctions.v1.projects.locations.functions.call
      args:
        name: "projects/${var.project_id}/locations/${var.location}/functions/${var.function_name}"
        data: ${google_json_encode({
          "url": "${google_workflows_workflow.example_workflow.outputs['step1.http_response.body']}"
        })}
EOF

  project_id = var.project_id
  location = var.location
  service_account_email = var.service_account_email
  description = "Example workflow that triggers a Cloud Function"
}


