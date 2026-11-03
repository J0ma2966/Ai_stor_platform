# Ansible: подготовка серверов

Один раз на новые серверы (Ubuntu 24.04 LTS / Debian 12): обновления, SSH только по ключам,
fail2ban, Docker, пользователь `deploy` для CD, приватная сеть WireGuard между Узбекистаном и
Германией и firewall. Подробности и порядок — [docs/DEPLOY.md](../docs/DEPLOY.md).

```bash
cp inventory.example.yml inventory.yml          # впишите IP серверов и SSH-ключи
ansible-galaxy collection install -r requirements.yml
ansible-playbook -i inventory.yml site.yml
```

С Windows проще запустить из WSL или из GitHub Actions (workflow `infra`, см. docs/DEPLOY.md).
