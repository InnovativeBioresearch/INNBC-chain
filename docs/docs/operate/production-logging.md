<!-- START ALPHA CHANGE 1476 -- add production logging and disk safety guide -->

# Production Logging and Disk Safety

This document defines the required logging and disk-safety configuration for production INNBC validator nodes and peer-directory services deployed with systemd.

Complete this procedure before leaving INNBC services running unattended.

## Why This Is Critical

INNBC services write operational messages to standard output and standard error. Under the packaged systemd deployment, systemd-journald collects those messages.

On servers that also run rsyslog, the same message may be stored twice:

```text
copy 1:

innbc-chain stdout/stderr
→ systemd-journald
→ /var/log/journal

copy 2:

innbc-chain stdout/stderr
→ system journal or syslog socket
→ rsyslog
→ /var/log/syslog
```

These are independent logging systems:

```text
systemd-journald:
  stores binary journal files
  is configured through journald.conf
  reports usage through journalctl --disk-usage

rsyslog:
  writes conventional text log files
  is configured through rsyslog.conf and rsyslog.d
  relies on logrotate for file rotation
```

A journald storage limit does not limit `/var/log/syslog`.

For example:

```ini
[Journal]
SystemMaxUse=1G
```

limits journald storage, but it does not stop rsyslog from independently growing `/var/log/syslog`.

Similarly:

```ini
MaxRetentionSec=30day
```

controls journal retention time. It does not impose a retention period or size limit on rsyslog files.

## Observed Failure

During development, a legacy consensus synchronization race caused one validator to enter an immediate retry loop.

The validator produced approximately:

```text
46,000 log records per minute
```

The repeated error caused:

```text
innbc-chain:
  approximately one full CPU core consumed

systemd-journald:
  increased processing load

rsyslog:
  increased processing load

/var/log/syslog:
  approximately 59 GB consumed
```

The node databases themselves were approximately:

```text
node1: 561 MB
node2: 561 MB
```

The duplicated syslog therefore became more than one hundred times larger than an individual node database.

Journald remained near its configured maximum because it had an effective storage cap. `/var/log/syslog` continued growing because it was controlled by a separate weekly logrotate policy without adequate containment.

The underlying consensus race has been corrected. The logging configuration must still be hardened independently because no application-level fix can guarantee that a future RPC, networking, storage, consensus, operating-system, or dependency failure will never produce another high-rate error.

## Required Production Baseline

A production server should use this model:

```text
INNBC validator and peer-directory logs:
  retained in systemd-journald
  excluded from duplicate rsyslog storage

systemd-journald:
  persistent storage capped at 1 GB
  older entries automatically discarded as required

remaining rsyslog facility logs:
  retained only where operationally useful
  rotated daily
  protected by a 1 GB rotation threshold

general /var/log/syslog catch-all:
  disabled after confirming that no installed service consumes it

validator services:
  only intended validators enabled
  unused validator instances stopped and disabled
```

The known consensus failure is fixed in the application. These logging controls provide defense in depth if any other component begins producing messages unexpectedly.

## 1. Inspect Existing Log Usage

Check journald usage:

```bash
sudo journalctl --disk-usage
```

Check the principal log paths:

```bash
sudo du -h \
  /var/log/syslog \
  /var/log/syslog.1 \
  /var/log/journal \
  2>/dev/null
```

Check total log-directory usage:

```bash
sudo du -sh /var/log
```

Check filesystem capacity:

```bash
df -h /
```

These measurements distinguish chain-data growth from operating-system log growth.

## 2. Inspect The Existing Journald Configuration

Show configured storage and rate limits:

```bash
sudo grep -RhsE \
'^[[:space:]]*(Storage|SystemMaxUse|SystemKeepFree|SystemMaxFileSize|MaxRetentionSec|RateLimitIntervalSec|RateLimitBurst)=' \
/etc/systemd/journald.conf \
/etc/systemd/journald.conf.d/*.conf \
2>/dev/null
```

A typical pre-hardening result may contain:

```text
SystemMaxUse=2G
MaxRetentionSec=30day
```

This limits journald only. It says nothing about the size of `/var/log/syslog`.

## 3. Inspect Rsyslog Rotation

Show the active rsyslog logrotate policy:

```bash
sudo sed -n '1,120p' /etc/logrotate.d/rsyslog
```

Confirm that the logrotate timer is enabled and active:

```bash
systemctl is-enabled logrotate.timer
systemctl is-active logrotate.timer
```

An unsafe configuration for a high-volume service may contain:

```text
rotate 4
weekly
```

without a `maxsize` threshold.

A weekly interval allows a high-rate process to generate an extremely large file before the next normal rotation.

## 4. Identify The Main Log Producer

When `/var/log/syslog` is unexpectedly large, sample only a bounded number of recent lines:

```bash
sudo tail -n 100000 /var/log/syslog |
awk '{
  program=$5
  sub(/\[[0-9]+\]:?$/, "", program)
  sub(/:$/, "", program)
  count[program]++
}
END {
  for (program in count) {
    print count[program], program
  }
}' |
sort -nr |
head -n 10
```

This counts the program identifiers in the last 100,000 lines without printing the entire file.

Do not run an unbounded `cat`, `grep`, or journal export against a multi-gigabyte incident log unless full-file processing is specifically required.

Count recent messages for one node without printing them:

```bash
sudo journalctl \
  -u innbc-node@node1.service \
  --since '1 minute ago' \
  --no-pager |
wc -l
```

A rate of thousands of records per minute is an application-health incident as well as a disk-consumption incident.

## 5. Stop A Log Flood Before Investigating Further

If INNBC is actively flooding logs, stop only the affected services first.

Stop one validator:

```bash
sudo systemctl stop innbc-node@node1.service
```

Stop selected validators:

```bash
sudo systemctl stop \
  innbc-node@node1.service \
  innbc-node@node2.service
```

Stop the peer-directory without changing its boot-time enablement:

```bash
sudo systemctl stop innbc-peerdirectory.service
```

For a server-wide INNBC incident, stop all configured validators and the peer-directory:

```bash
sudo systemctl stop \
  innbc-node@node1.service \
  innbc-node@node2.service \
  innbc-node@node3.service \
  innbc-node@node4.service \
  innbc-node@node5.service \
  innbc-node@node6.service \
  innbc-peerdirectory.service
```

Verify current runtime states:

```bash
systemctl is-active innbc-node@node{1..6}.service
systemctl is-active innbc-peerdirectory.service
```

Understand the difference between service states:

```text
active:
  the service is running now

inactive:
  the service is not running now

enabled:
  the service is configured to start automatically at boot

disabled:
  the service is not configured to start automatically at boot
```

Stopping a service does not disable it.

Check boot-time enablement separately:

```bash
systemctl is-enabled innbc-node@node{1..6}.service
systemctl is-enabled innbc-peerdirectory.service
```

To stop and disable an unintended validator:

```bash
sudo systemctl disable --now innbc-node@node3.service
```

Only validators deliberately intended to run should be enabled.

## 6. Keep INNBC Logs In Journald Only

Create an early rsyslog rule that stops processing INNBC messages before they reach the normal catch-all rules:

```bash
sudo tee /etc/rsyslog.d/20-innbc-chain-journal-only.conf >/dev/null <<'EOF'
if $programname == 'innbc-chain' then stop
EOF
```

The validator and peer-directory services currently use the same `innbc-chain` executable. Their observed rsyslog program name is:

```text
innbc-chain
```

The configuration filename begins with `20-` so it is normally processed before Ubuntu’s `50-default.conf`.

If future systemd units define a different `SyslogIdentifier`, update and retest this rule.

Validate the complete rsyslog configuration:

```bash
sudo rsyslogd -N1
```

Do not restart rsyslog if validation reports an error.

Successful validation ends with:

```text
End of config validation run. Bye.
```

Activate the rule:

```bash
sudo systemctl restart rsyslog
```

## 7. Verify The INNBC Rsyslog Exclusion

Generate an INNBC-tagged test message:

```bash
logger -t innbc-chain 'INNBC_RSYSLOG_FILTER_TEST'
```

Confirm that journald retained it:

```bash
sudo journalctl \
  -t innbc-chain \
  --since '1 minute ago' \
  --no-pager
```

Confirm that `/var/log/syslog` did not receive it:

```bash
sudo tail -n 100 /var/log/syslog |
grep 'INNBC_RSYSLOG_FILTER_TEST' ||
echo 'PASS: INNBC message was not copied to syslog'
```

The required result is:

```text
journald:
  test message present

/var/log/syslog:
  test message absent
```

Do not delete an oversized syslog until this test passes.

## 8. Cap Persistent Journald Storage At 1 GB

Create the journald drop-in directory:

```bash
sudo install -d -m 0755 /etc/systemd/journald.conf.d
```

Create the storage override:

```bash
sudo tee /etc/systemd/journald.conf.d/20-storage-limit.conf >/dev/null <<'EOF'
[Journal]
SystemMaxUse=1G
EOF
```

This is a global journald limit for the server, not an INNBC-only limit.

Restart journald:

```bash
sudo systemctl restart systemd-journald
```

Rotate the active journal:

```bash
sudo journalctl --rotate
```

Reduce archived journal storage to the configured target:

```bash
sudo journalctl --vacuum-size=1G
```

Warning: journal vacuuming permanently removes the oldest archived journal entries. Preserve any required security, consensus, or incident evidence before vacuuming.

Verify current usage:

```bash
sudo journalctl --disk-usage
```

A successful result should be approximately:

```text
Archived and active journals take up 1.0G in the file system.
```

Small implementation-dependent variation is normal because active journal files and filesystem allocation occur in blocks.

## 9. Rotate Remaining Rsyslog Files Daily

Edit:

```text
/etc/logrotate.d/rsyslog
```

The active rsyslog stanza should use:

```text
{
rotate 4
daily
maxsize 1G
missingok
notifempty
compress
delaycompress
sharedscripts
postrotate
/usr/lib/rsyslog/rsyslog-rotate
endscript
}
```

The important changes are:

```text
weekly      → daily
no maxsize  → maxsize 1G
```

The directives mean:

```text
rotate 4:
  retain four rotated generations

daily:
  rotate eligible files daily rather than weekly

maxsize 1G:
  allow early rotation when logrotate observes a file over 1 GB

compress:
  compress eligible older rotations

delaycompress:
  leave the newest rotated generation uncompressed until the next rotation

missingok:
  do not fail because an optional log file is absent

notifempty:
  do not rotate an empty file

sharedscripts:
  execute the postrotate action once for the complete group
```

`maxsize 1G` is not a byte-perfect real-time quota. Logrotate checks the condition only when it runs.

The INNBC-specific rsyslog exclusion is therefore the primary protection against node log floods. Daily rotation and `maxsize` provide secondary containment for the remaining rsyslog files.

Confirm the final policy:

```bash
sudo sed -n '1,120p' /etc/logrotate.d/rsyslog
```

## 10. Audit Consumers Of `/var/log/syslog`

Before disabling the general `/var/log/syslog` catch-all, check whether installed security or monitoring software explicitly consumes it:

```bash
sudo grep -Rsl '/var/log/syslog' \
/etc/fail2ban \
/etc/webmin \
/etc/monit \
/etc/zabbix \
/etc/prometheus \
2>/dev/null
```

No output means that none of those inspected configurations explicitly references `/var/log/syslog`.

Extend the audit when the server contains additional:

- intrusion-detection software;
- log shippers;
- SIEM agents;
- compliance software;
- monitoring agents;
- custom maintenance scripts.

Locate the exact rsyslog rule writing `/var/log/syslog`:

```bash
sudo grep -Rns '/var/log/syslog' \
/etc/rsyslog.conf \
/etc/rsyslog.d \
2>/dev/null
```

A common Ubuntu result is:

```text
/etc/rsyslog.d/50-default.conf:9:*.*;auth,authpriv.none         -/var/log/syslog
```

## 11. Optionally Disable Only The General Catch-All

After confirming that nothing requires `/var/log/syslog`, comment the catch-all rule:

```text
# *.*;auth,authpriv.none         -/var/log/syslog
```

Validate the resulting configuration:

```bash
sudo rsyslogd -N1
```

Activate it:

```bash
sudo systemctl restart rsyslog
```

This does not require disabling rsyslog.

Facility-specific logging may remain active:

```text
auth,authpriv.*    /var/log/auth.log
kern.*             /var/log/kern.log
mail.*             /var/log/mail.log
```

Keeping these files can preserve useful authentication, kernel, and mail diagnostics while avoiding a general duplicate of journald.

Do not disable rsyslog entirely unless every rsyslog-managed log and every consumer has been audited.

## 12. Recover Space From An Existing Oversized Syslog

Before deleting the oversized file:

1. Stop the process producing the flood.
2. Preserve the necessary diagnostic evidence.
3. Install the INNBC rsyslog exclusion.
4. Validate and restart rsyslog.
5. Confirm that an INNBC test message remains journal-only.
6. Only then remove the obsolete file contents.

If an incident excerpt must be retained, capture a bounded tail:

```bash
sudo tail -n 10000 /var/log/syslog \
  > /tmp/innbc-syslog-incident-tail.txt
```

Review and move that file to an appropriately protected incident-storage location if it must be kept.

To reclaim the current `/var/log/syslog` allocation:

```bash
sudo truncate -s 0 /var/log/syslog
```

Warning: this permanently deletes all current contents of `/var/log/syslog`. It is not a rotation or an archive operation.

Rsyslog can continue running because the file itself remains present and its open file descriptor remains valid.

Confirm that the space was reclaimed:

```bash
sudo du -h /var/log/syslog
df -h /
```

A successful truncation reports:

```text
0    /var/log/syslog
```

## 13. Verify The Final Logging Policy

Check journald:

```bash
sudo journalctl --disk-usage
```

Check `/var/log/syslog`:

```bash
sudo stat -c '%s bytes in /var/log/syslog' /var/log/syslog
```

Send a general test message:

```bash
logger -t RSYSLOG_CATCHALL_TEST \
  'general message should remain journal-only'
```

Confirm that journald retained it:

```bash
sudo journalctl \
  -t RSYSLOG_CATCHALL_TEST \
  -n 1 \
  --no-pager
```

Confirm that the disabled catch-all did not copy it:

```bash
sudo stat -c '%s bytes in /var/log/syslog' /var/log/syslog
```

Inspect remaining facility-specific files:

```bash
sudo du -h \
  /var/log/auth.log \
  /var/log/kern.log \
  /var/log/mail.log \
  2>/dev/null
```

Check all INNBC service states:

```bash
systemctl is-active innbc-node@node{1..6}.service
systemctl is-active innbc-peerdirectory.service
```

Check boot-time enablement:

```bash
systemctl is-enabled innbc-node@node{1..6}.service
systemctl is-enabled innbc-peerdirectory.service
```

## 14. Inspect Logs Safely

Follow one validator:

```bash
sudo journalctl \
  -u innbc-node@node1.service \
  -f
```

Follow the peer-directory:

```bash
sudo journalctl \
  -u innbc-peerdirectory.service \
  -f
```

Show only recent warnings and errors:

```bash
sudo journalctl \
  -u innbc-node@node1.service \
  --since '10 minutes ago' \
  -p warning \
  --no-pager |
tail -n 100
```

Count a repeated error without printing every occurrence:

```bash
sudo journalctl \
  -u innbc-node@node1.service \
  --since '1 minute ago' \
  --no-pager |
grep -c 'error text'
```

Find the first recent occurrence of a known error:

```bash
sudo journalctl \
  -u innbc-node@node1.service \
  --since '24 hours ago' \
  --grep='error text' \
  -n 1 \
  -o short-iso \
  --no-pager
```

Always constrain investigation commands by at least one of:

- systemd unit;
- time interval;
- priority;
- match expression;
- line count.

Avoid unbounded terminal output during a high-rate incident.

## 15. Ongoing Monitoring

Regularly check:

```bash
sudo journalctl --disk-usage
sudo du -sh /var/log
df -h /
```

A useful operational baseline includes monitoring for:

- filesystem utilization;
- journal utilization;
- unusually rapid log growth;
- repeated identical errors;
- high CPU usage by `innbc-chain`;
- high CPU usage by `systemd-journald`;
- high CPU usage by `rsyslogd`;
- unexpected validator services;
- validators enabled before admission;
- failed or repeatedly restarting services.

Check high-CPU processes with:

```bash
ps -eo \
pid,ppid,user,comm,%cpu,%mem,etime,args \
--sort=-%cpu |
head -n 20
```

Check current INNBC processes:

```bash
pgrep -af 'innbc-chain server'
```

Check recent service restart information:

```bash
systemctl show innbc-node@node1.service \
  -p ActiveState \
  -p SubState \
  -p MainPID \
  -p NRestarts
```

## 16. Incident Interpretation

A log cap prevents disk exhaustion, but it does not correct the failure generating the messages.

Treat a sudden log-rate increase as two simultaneous incidents:

```text
application incident:
  why is the node repeating the message?

infrastructure incident:
  can the logging pipeline exhaust disk, CPU, or I/O?
```

The application should:

- avoid tight retry loops;
- apply retry backoff where recovery is possible;
- throttle repeated equivalent messages;
- propagate fatal invariant failures to the service owner;
- terminate cleanly when safe progress is impossible;
- rely on the configured supervisor for controlled restart.

The infrastructure should:

- prevent duplicate retention;
- enforce bounded journal storage;
- rotate required flat files;
- alert on disk usage and log rate;
- preserve enough diagnostics for investigation;
- prevent a single process from exhausting the host filesystem.

Neither layer replaces the other.

## 17. Production Checklist

Before enabling a validator:

- [ ] Only intended validator instances are enabled.
- [ ] Unused validator instances are stopped and disabled.
- [ ] The peer-directory state is intentional.
- [ ] INNBC messages appear in journald.
- [ ] INNBC messages do not appear in `/var/log/syslog`.
- [ ] Journald has an explicit storage maximum.
- [ ] `journalctl --disk-usage` reports acceptable usage.
- [ ] Rsyslog configuration passes `rsyslogd -N1`.
- [ ] Remaining rsyslog files rotate daily.
- [ ] Remaining rsyslog files have a maximum-size threshold.
- [ ] `/var/log/syslog` consumers have been audited.
- [ ] Authentication and kernel logging remain available where required.
- [ ] Root filesystem utilization is monitored.
- [ ] Node CPU usage is monitored.
- [ ] Repeated-error rates are monitored.
- [ ] Incident logs are captured with bounded commands.

The logging configuration is complete only when both retention and routing have been verified on the actual production server.

<!-- FINISH ALPHA CHANGE 1476 -- add production logging and disk safety guide -->