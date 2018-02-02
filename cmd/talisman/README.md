### Running talisman

To test talisman, you can use the talisman_job.yaml spec file in this directory.

```bash
kubectl apply -f talisman_job.yaml
```

This will start a [Job](https://kubernetes.io/docs/concepts/workloads/controllers/jobs-run-to-completion/).

You can monitor the job using:

```bash
kubectl get -f talisman_job.yaml
kubectl describe -f talisman_job.yaml
```

### Talisman args

- Currently, the only operation supported is `upgrade` in the `-operation` arguments of the talisman Job.
- To change the upgrade image, change `-newimage`
