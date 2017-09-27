#!/bin/bash -eux

cat \
  <(head -n 17 "${BOSH_DEPLOYMENT_DIR}/external-ip-not-recommended.yml" \
  | sed s/"((external_ip))"/"((bosh_lite_external_ip))"/) \
  "${BOSH_DEPLOYMENT_DIR}/bosh-lite.yml" \
  "${BOSH_DEPLOYMENT_DIR}/bosh-lite-runc.yml" \
  <(tail -n +2 "${BOSH_DEPLOYMENT_DIR}/gcp/bosh-lite-vm-type.yml") \
  > bosh-lite-combined.yml

