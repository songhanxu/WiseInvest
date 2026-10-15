#!/usr/bin/env python3
"""Exercise deployment flow with fake commands; never contact Docker or a server."""
import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest


SCRIPT = Path(__file__).resolve().parents[1] / "deploy-update.sh"
MOCK = r'''#!/usr/bin/env python3
import json, os, shutil, sys, time
from pathlib import Path
name, args = Path(sys.argv[0]).name, sys.argv[1:]
with open(os.environ["MOCK_LOG"], "a") as f:
    f.write(json.dumps([name, *args]) + "\n")
if name == "sudo":
    os.environ["MOCK_SUDO"] = "1"
    os.execv(shutil.which(args[0]), args)
if name == "sleep":
    time.sleep(0.05)
    sys.exit(0)
if name == "curl":
    assert "-fsS" in args, "HTTP failures must be rejected"
    log = Path(os.environ["MOCK_LOG"]).read_text()
    calls = sum(json.loads(line)[0] == "curl" for line in log.splitlines())
    proxy_failed = os.environ.get("MOCK_PROXY_FAILURE") and args[-1] == "http://127.0.0.1/health"
    sys.exit(22 if os.environ.get("MOCK_UNHEALTHY") or proxy_failed or (os.environ.get("MOCK_RETRY") and calls == 1) else 0)
if name == "docker" and args == ["info"]:
    sys.exit(1 if os.environ.get("MOCK_NEED_SUDO") and not os.environ.get("MOCK_SUDO") else 0)
if name == "docker" and args[:1] == ["inspect"]:
    print("false" if args[-1] == os.environ.get("MOCK_STOPPED") else "true")
    sys.exit(0)
if name == "docker":
    assert args.pop(0) == "compose", args
if args == ["version"]:
    sys.exit(1 if name == "docker" and os.environ.get("MOCK_V1") else 0)
assert args[0] == "-f" and args[2] == "--project-directory", args
assert Path(args[1]).is_file() and Path(args[3]).is_dir(), args
args = args[4:]
if args[:2] == ["ps", "-q"]:
    print("container-" + args[-1])
elif args == ["build", "backend"]:
    sys.exit(2 if os.environ.get("MOCK_BUILD_FAILURE") else 0)
elif args == ["up", "-d", "--no-deps", "--force-recreate", "backend"]:
    if os.environ.get("MOCK_LEGACY_BUG") and not Path(os.environ["MOCK_LOG"] + ".removed").exists():
        print("KeyError: 'ContainerConfig'", file=sys.stderr)
        sys.exit(1)
elif args == ["rm", "-s", "-f", "backend"]:
    Path(os.environ["MOCK_LOG"] + ".removed").touch()
elif args[:1] == ["exec"]:
    assert args in [["exec", "-T", "nginx", "nginx", "-t"], ["exec", "-T", "nginx", "nginx", "-s", "reload"]], args
    if os.environ.get("MOCK_NGINX_FAILURE"):
        sys.exit(1)
elif args in [["logs", "--tail=80", "backend"], ["ps", "backend", "nginx"]]:
    pass
else:
    raise AssertionError(args)
'''


class DeployUpdateTests(unittest.TestCase):
    def run_deploy(self, *, missing_env=False, **flags):
        with tempfile.TemporaryDirectory(prefix="wiseinvest deploy test ") as tmp:
            root = Path(tmp)
            project, commands = root / "project with spaces", root / "commands"
            project.mkdir()
            commands.mkdir()
            (project / "backend").mkdir()
            (project / "docker-compose.yml").write_text("services: {}\n")
            production = project / "backend/.env.production"
            secret = "FMP_API_KEY=must-not-print-this-value\n"
            if not missing_env:
                production.write_text(secret)
            shutil.copyfile(SCRIPT, project / SCRIPT.name)
            for name in ("docker", "docker-compose", "sudo", "curl", "sleep"):
                path = commands / name
                path.write_text(MOCK)
                path.chmod(0o755)
            log = root / "commands.jsonl"
            env = dict(os.environ, PATH=str(commands) + os.pathsep + os.environ["PATH"],
                       MOCK_LOG=str(log), DEPLOY_HEALTH_TIMEOUT="1", **flags)
            result = subprocess.run(["bash", str(project / SCRIPT.name)], cwd=root,
                                    env=env, text=True, capture_output=True, timeout=15)
            calls = [json.loads(line) for line in log.read_text().splitlines()] if log.exists() else []
            if not missing_env:
                self.assertEqual(production.read_text(), secret)
            self.assertNotIn("must-not-print-this-value", result.stdout + result.stderr)
            return result, calls

    def test_success_v2_v1_and_sudo(self):
        for flags in ({}, {"MOCK_V1": "1"}, {"MOCK_NEED_SUDO": "1"}):
            with self.subTest(flags=flags):
                result, calls = self.run_deploy(**flags)
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertIn("更新成功", result.stdout)
                build = next(i for i, c in enumerate(calls) if c[-2:] == ["build", "backend"])
                up = next(i for i, c in enumerate(calls) if "--force-recreate" in c)
                reload = next(i for i, c in enumerate(calls) if c[-2:] == ["-s", "reload"])
                health = [i for i, c in enumerate(calls) if c[0] == "curl"]
                self.assertLess(build, up)
                self.assertLess(up, health[0])
                self.assertLess(health[0], reload)
                self.assertLess(reload, health[-1])

    def test_missing_configuration_stops_before_docker(self):
        result, calls = self.run_deploy(missing_env=True)
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(calls, [])

    def test_stopped_dependency_stops_before_build(self):
        result, calls = self.run_deploy(MOCK_STOPPED="container-postgres")
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse(any("build" in c for c in calls))

    def test_build_failure_preserves_running_backend(self):
        for flags in ({}, {"MOCK_V1": "1", "MOCK_LEGACY_BUG": "1"}):
            with self.subTest(flags=flags):
                result, calls = self.run_deploy(MOCK_BUILD_FAILURE="1", **flags)
                self.assertNotEqual(result.returncode, 0)
                self.assertFalse(any("up" in c or "rm" in c for c in calls))
                self.assertTrue(any("logs" in c for c in calls))

    def test_legacy_containerconfig_failure_avoided_after_build(self):
        result, calls = self.run_deploy(MOCK_V1="1", MOCK_LEGACY_BUG="1")
        self.assertEqual(result.returncode, 0, result.stderr)
        build = next(i for i, c in enumerate(calls) if c[-2:] == ["build", "backend"])
        remove = next(i for i, c in enumerate(calls) if c[-4:] == ["rm", "-s", "-f", "backend"])
        up = next(i for i, c in enumerate(calls) if "--force-recreate" in c)
        self.assertLess(build, remove)
        self.assertLess(remove, up)
        self.assertFalse(any("-v" in c or "down" in c for c in calls))

    def test_modern_compose_keeps_normal_recreation(self):
        result, calls = self.run_deploy()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertFalse(any("rm" in c for c in calls))

    def test_unhealthy_backend_does_not_reload_nginx(self):
        result, calls = self.run_deploy(MOCK_UNHEALTHY="1")
        self.assertNotEqual(result.returncode, 0)
        self.assertNotIn("更新成功", result.stdout)
        self.assertFalse(any("reload" in c for c in calls))
        self.assertTrue(any("logs" in c for c in calls))

    def test_transient_health_failure_is_retried(self):
        result, calls = self.run_deploy(MOCK_RETRY="1")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(sum(c[0] == "curl" for c in calls), 3)

    def test_nginx_and_proxy_failures_are_not_success(self):
        for flag in ("MOCK_NGINX_FAILURE", "MOCK_PROXY_FAILURE"):
            with self.subTest(flag=flag):
                result, calls = self.run_deploy(**{flag: "1"})
                self.assertNotEqual(result.returncode, 0)
                self.assertNotIn("更新成功", result.stdout)
                self.assertTrue(any("logs" in c for c in calls))


if __name__ == "__main__":
    unittest.main()
