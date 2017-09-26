#!/bin/bash -eu

bosh_lite_external_ip="$(jq <terraform/terraform.tfstate '.modules[0].outputs.bosh_lite_external_ip.value' -r)"

cat \
  <(head -n 17 bosh/director/gcp-external-ip-not-recommended.yml \
  | sed s/"((external_ip))"/"${bosh_lite_external_ip}"/) \
  "${BOSH_DEPLOYMENT_DIR}/bosh-lite.yml" \
  "${BOSH_DEPLOYMENT_DIR}/bosh-lite-runc.yml" \
  <(tail -n +2 "${BOSH_DEPLOYMENT_DIR}/gcp/bosh-lite-vm-type.yml") \
  > bosh-lite-combined.yml

