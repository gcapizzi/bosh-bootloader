# TL;DR:

Here's what we were able to do to get a working bosh-lite using the latest `bbl`:

1. Check out the `write-to-state-dir` branch of `bbl`.
1. Add the terraform override file below as `override.tf` into a `terraform/` directory in your state-dir.
1. Run `bbl up` once without using any ops files.
1. Run the script below with the environment variable `BOSH_DEPLOYMENT_DIR=(location of your bosh-deployment dir)`.
1. Run `bbl up --ops-file bosh-lite-combined.yml`.
1. Use `eval "$(bbl print-env)"` to target your bosh director.
1. Follow the remaining steps (5-7) in [this doc](https://github.com/cloudfoundry/cf-deployment/blob/master/bosh-lite.md#5-upload-the-cloud-config).
1. Add DNS records for our CF instance, eg:
```
bosh-lite.infrastructure.cf-app.com.	A	300	${bosh_lite_external_ip}
*.bosh-lite.infrastructure.cf-app.com.	CNAME	300	bosh-lite.infrastructure.cf-app.com.
```
1. Target your CF instance and push an app.


What still sucks about this?

1. You have to `bbl up` twice.
   We need to get the external IP address from terraform before we can provide it to the ops file.
2. You can't just write the IP address into `bosh-deployment-vars.yml`, you have to write it directly into the ops file.
   The vars file gets regenerated each time before bbl runs `bosh interpolate`.


### Terraform override

```
resource "google_compute_firewall" "bosh-director-lite" {
  name = "${var.env_id}-bosh-director-lite"
  network = "${google_compute_network.bbl-network.name}"

  source_ranges = ["0.0.0.0/0"]

  allow {
    ports = ["80", "443", "2222"]
    protocol = "tcp"
  }

  target_tags = ["${var.env_id}-bosh-director"]
}

resource "google_compute_address" "bosh-lite-ip" {
  name = "${var.env_id}-bosh-lite-ip"
}

output "bosh_lite_external_ip" {
    value = "${google_compute_address.bosh-lite-ip.address}"
}
```

### Script for creating bosh lite ops file

Run with `BOSH_DEPLOYMENT_DIR=(location of your bosh-deployment dir)`:
```
bosh_lite_external_ip="$(jq <terraform/terraform.tfstate '.modules[0].outputs.bosh_lite_external_ip.value' -r)"

cat \
  <(head -n 17 bosh/director/gcp-external-ip-not-recommended.yml \
  | sed s/"((external_ip))"/"${bosh_lite_external_ip}"/) \
  "${BOSH_DEPLOYMENT_DIR}/bosh-lite.yml" \
  "${BOSH_DEPLOYMENT_DIR}/bosh-lite-runc.yml" \
  <(tail -n +2 "${BOSH_DEPLOYMENT_DIR}/gcp/bosh-lite-vm-type.yml") \
  > bosh-lite-combined.yml
```




## Notes on bosh-lite exploration

The following are just notes for all the things we tried along the way...

### First thing we tried:

1. We started with the `write-to-state-dir` branch, which outputs most of the stuff to the state dir.
1. We bbl'd up
1. Then we added an `override.tf` file to the `terraform/` directory in the state dir
   that opens up the ports to access the bosh director directly.
   We also added an output for the external IP address for the bosh director.
1. Then, we concatentated the ops files noted [here](https://github.com/cloudfoundry/cf-deployment/blob/master/bosh-lite.md),
   along with `external-ip-not-recommended.yml`
1. We tried to bbl up again

We got an error in the bosh interpolate step where it was adding our ops file:

```
Expected to find exactly one matching array item for path '/variables/name=mbus_bootstrap_ssl' but found 0
```

We tried commenting out those files in bosh executor, but still got the same error.

```
uaa.yml
credhub.yml
gcp-bosh-director-ephemeral-ip-ops.yml
```

When we added the `external-ip-not-recommended` in bosh executor itself, that got past the above error,
but we then got an error because our variable was not present for the external IP:

It gets further because the `variables` section of the manifest is only there until
interpolate is run with the `--vars-store` flag.

```
- Expected to find variables:
    - external_ip
```

We tried adding this variable to the `bosh/director/deployments-vars.yml` file, but the file
always gets overwritten by bbl.



### Next thing we tried:

1. We started with the `write-to-state-dir` branch, which outputs most of the stuff to the state dir.
1. We used the same `override.tf` file in the `terraform/` directory from our first attempt.
1. We concatenated the bosh-lite ops files per [these instructions](https://github.com/cloudfoundry/cf-deployment/blob/master/bosh-lite.md)
1. We bbl'd up with the ops file: `bbl up --ops-file bosh-lite-concatenated.yml`
1. We had one flake where bbl up failed, but it worked on the second try
1. We updated the cloud-config with the file in `cf-deployment/bosh-lite/cloud-config.yml`
1. We tried to upload a stemcell: `bosh upload-stemcell https://bosh.io/d/stemcells/bosh-warden-boshlite-ubuntu-trusty-go_agent`

But we got this error:

```
Task 1 | 19:11:59 | Update stemcell: Uploading stemcell bosh-warden-boshlite-ubuntu-trusty-go_agent/3445.11 to the cloud (00:00:00)
                   L Error: CPI error 'Bosh::Clouds::CloudError' with message 'Creating stemcell: Invalid 'warden' infrastructure' in 'create_stemcell' CPI method
```

Interestingly, this returns nothing currently: `bosh cpi-config`
(That is a red herring... )

This issue was that the concatenated ops file had a line with `---` halfway through it, and everything above it was ignored.


### Third try

1. We started with the `write-to-state-dir` branch, which outputs most of the stuff to the state dir.
1. We used the same `override.tf` file in the `terraform/` directory from our first attempt.
1. We constructed a script to concatenate the ops file without the `---` line, and including the external IP for the BOSH lite VM generated by Terraform.
1. We ran `bbl up` against our script-generated ops file.
1. We updated the cloud-config with the file in `cf-deployment/bosh-lite/cloud-config.yml`
1. We uploaded a stemcell: `bosh upload-stemcell https://bosh.io/d/stemcells/bosh-warden-boshlite-ubuntu-trusty-go_agent`
1. We deployed CF.
1. We manually added DNS records for our CF instance.
1. We targeted our CF instance and pushed an app.
