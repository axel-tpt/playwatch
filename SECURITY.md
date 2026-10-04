# Security policy

## Supported versions

playwatch is in early development (0.x). Only the latest release receives security fixes.

## Reporting a vulnerability

**Please do not report security issues in public issues, pull requests or discussions.**

Report them privately through GitHub: go to the [Security tab](https://github.com/axel-tpt/playwatch/security) of the repository and click **Report a vulnerability**.

Please include:

- the playwatch version (`playwatch --version`), Ansible version and OS,
- a description of the issue and its impact,
- steps to reproduce, ideally with a minimal playbook or inventory.

You should get a first answer within a week. Once the issue is confirmed, a fix will be prepared and released, and you will be credited in the release notes unless you prefer otherwise.

## Scope

playwatch handles sensitive data on behalf of Ansible. Issues in these areas are especially relevant:

- **Vault passwords**: a password typed in playwatch must only reach Ansible through a pipe, and must never be written to disk, logged or stored in the run history.
- **`no_log`**: results of tasks marked `no_log` must be censored in events, in the run history and in the UI.
- **Run history**: files under `$XDG_STATE_HOME/playwatch/runs` must only be readable by their owner.
- **Callback plugin**: the plugin written to a temporary directory and the event pipe passed to `ansible-playbook` must not be tampered with or leaked to other processes.

Vulnerabilities in Ansible itself should be reported to the [Ansible project](https://docs.ansible.com/ansible/latest/community/reporting_bugs_and_features.html).
