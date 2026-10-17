import 'package:dispatch_provider/core/storage/token_storage.dart';
import 'package:flutter_secure_storage/flutter_secure_storage.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();

  setUp(() {
    FlutterSecureStorage.setMockInitialValues({});
  });

  test('installation ID remains stable after clearing account tokens', () async {
    final storage = TokenStorage();
    final first = await storage.getOrCreateDeviceId();
    final second = await storage.getOrCreateDeviceId();
    expect(first, second);
    expect(first, startsWith('dp-'));
    expect(first.length, greaterThan(24));

    await storage.saveTokens(
      accessToken: 'access',
      refreshToken: 'refresh',
      providerId: 'provider-a',
    );
    await storage.clear();
    expect((await storage.readTokens()).isEmpty, isTrue);
    expect(await storage.getOrCreateDeviceId(), first);
  });

  test('a fresh installation generates a different device identifier',
      () async {
    final first = await TokenStorage().getOrCreateDeviceId();
    FlutterSecureStorage.setMockInitialValues({});
    final second = await TokenStorage().getOrCreateDeviceId();
    expect(second, isNot(first));
  });
}
