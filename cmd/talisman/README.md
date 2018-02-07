### Running talisman

To test talisman, you can run the run_px_upgrade.sh script. For example:

```bash
./run_px_upgrade.sh --ocimontag 1.3.0-rc4
```

This will start a [Job](https://kubernetes.io/docs/concepts/workloads/controllers/jobs-run-to-completion/). You can monitor the job logs using:

```bash
kubectl logs -n kube-system -l job-name=talisman -f
```

Run `./run_px_upgrade.sh --help` for more usage examples.

Note: Currently, the only operation supported is `upgrade` in the `-operation` arguments of the talisman Job.

