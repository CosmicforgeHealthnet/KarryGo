"""Read-only route mounting audit. Authentication and workflow verification are separate."""
import argparse
import concurrent.futures
import datetime
import json
import pathlib
import os
import shutil
import subprocess
import urllib.error
import urllib.request


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--origin', required=True)
    parser.add_argument('--output', required=True)
    args = parser.parse_args()
    origin = args.origin.rstrip('/')
    if not origin.startswith('https://'):
        parser.error('Use an explicit HTTPS origin.')
    root = pathlib.Path(__file__).resolve().parents[1]
    manifest = json.loads((root / 'contracts/vendor_manifest.json').read_text(encoding='utf-8'))
    groups = {}
    for operation in manifest['operations']:
        groups.setdefault(operation['module'], []).append(operation)

    def check(item):
        module, operations = item
        reads = [op for op in operations if op['method'] == 'GET' and '{' not in op['path']]
        if not reads:
            return {'module': module, 'operations': len(operations), 'result': 'no-parameter-free-read'}
        path = min(reads, key=lambda op: len(op['path']))['path']
        curl = shutil.which('curl.exe') or shutil.which('curl')
        if curl:
            response = subprocess.run([curl, '--silent', '--show-error',
                '--max-time', '15', '--proto', '=https', '--output', os.devnull,
                '--write-out', '%{http_code}', origin + path], capture_output=True, text=True)
            if response.returncode:
                return {'module': module, 'path': path, 'operations': len(operations),
                        'result': 'connection-error', 'detail': response.stderr.strip()}
            status = int(response.stdout)
        else:
            request = urllib.request.Request(origin + path, headers={'Accept': 'application/json'})
            try:
                with urllib.request.urlopen(request, timeout=15) as response:
                    status = response.status
            except urllib.error.HTTPError as error:
                status = error.code
            except (urllib.error.URLError, TimeoutError, OSError) as error:
                return {'module': module, 'path': path, 'operations': len(operations),
                        'result': type(error).__name__, 'detail': str(error)}
        result = {401: 'authentication-required', 403: 'access-refused', 404: 'route-not-found'}.get(status, 'inspect-response')
        return {'module': module, 'operations': len(operations), 'path': path, 'http_status': status, 'result': result}

    with concurrent.futures.ThreadPoolExecutor(max_workers=4) as pool:
        results = sorted(pool.map(check, groups.items()), key=lambda item: item['module'])
    report = {
        'checked_at': datetime.datetime.now(datetime.timezone.utc).isoformat(),
        'origin': origin, 'contract_operations': len(manifest['operations']),
        'scope': 'Unauthenticated read-only route mounting. Not authenticated end-to-end certification.',
        'modules': results,
    }
    output = pathlib.Path(args.output)
    output.parent.mkdir(parents=True, exist_ok=True)
    output.write_text(json.dumps(report, indent=2) + '\n', encoding='utf-8')
    for result in results:
        print(f"{result['module']}: {result.get('http_status', '-')} {result['result']}")


if __name__ == '__main__':
    main()
