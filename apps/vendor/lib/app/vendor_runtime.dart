import 'dart:async';
import 'package:flutter/foundation.dart';
import 'package:http/http.dart' as http;
import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import '../core/config/vendor_app_config.dart';
import '../features/auth/data/vendor_session_store.dart';
import '../features/auth/state/vendor_auth_controller.dart';
import 'data/mock_vendor_client.dart';
import 'data/mock_onboarding_store.dart';
import 'data/mock_dashboard_data.dart';
import 'data/vendor_preferences.dart';
import 'vendor_app.dart';

enum VendorDataMode { mock, live }

enum VendorPage {
  splash,
  onboarding,
  signIn,
  code,
  vendorChoice,
  signedIn,
  accessUnavailable,
  connectionError,
}

class VendorRuntime extends ChangeNotifier {
  VendorRuntime({
    VendorPreferences? preferences,
    this.liveSessionStore,
    this.liveClientFactory,
    this.mockLatency = const Duration(milliseconds: 450),
  }) : preferences = preferences ?? VendorPreferences();
  final VendorPreferences preferences;
  final VendorPreferences mockSupportPreferences = VendorPreferences(
    inMemory: true,
  );
  final VendorSessionStore? liveSessionStore;
  final http.Client Function()? liveClientFactory;
  final Duration mockLatency;
  bool balancesVisible = true;
  void toggleBalances() {
    balancesVisible = !balancesVisible;
    _emit();
  }

  VendorDataMode mode = VendorDataMode.mock;
  MockDashboardScenario dashboardScenario = MockDashboardScenario.populated;
  MockAuthScenario scenario = MockAuthScenario.multipleVendors;
  MockOnboardingScenario onboardingScenario = MockOnboardingScenario.draft;
  VendorPage page = VendorPage.splash;
  String liveOrigin = const String.fromEnvironment('VENDOR_API_ORIGIN');
  String identifier = '';
  bool useEmail = false;
  String? error;
  String? selectedVendorId;
  DateTime? _expiresAt, _retryAt;
  VendorApp? _app;
  http.Client? _transport;
  Timer? _ticker;
  bool _working = false, _disposed = false;
  int revision = 0;
  Future<void> Function()? _retry;
  VendorPage _retryPage = VendorPage.signIn;

  http.Client? get mockUploadTransport =>
      mode == VendorDataMode.mock ? _transport : null;
  VendorApp get backend => _app!;
  VendorAuthController get auth => backend.auth;
  bool get busy => _working || (_app?.auth.busy ?? false);
  int get remainingSeconds => _secondsUntil(_expiresAt);
  int get retrySeconds => _secondsUntil(_retryAt);
  bool get codeExpired => _expiresAt != null && remainingSeconds == 0;
  bool get canRetry => _retry != null && canRequest;
  bool get canRequest => !busy && retrySeconds == 0;
  bool get suspended => error?.toLowerCase().contains('suspended') ?? false;
  String get maskedIdentifier {
    if (useEmail) {
      final parts = identifier.split('@');
      return '${parts.first.isEmpty ? '' : parts.first[0]}•••@${parts.length > 1 ? parts.last : ''}';
    }
    if (identifier.length < 5) return identifier;
    return '${identifier.substring(0, 4)} ••• ••• ${identifier.substring(identifier.length - 4)}';
  }

  Future<void> initialize() async {
    try {
      if (liveOrigin.isEmpty) {
        liveOrigin = await preferences.read('vendor.settings.origin') ?? '';
      }
    } catch (_) {
      error = 'Your saved settings could not be loaded.';
    }
    if (_disposed) return;
    await _connect();
  }

  Future<void> _connect() async {
    _working = true;
    page = VendorPage.splash;
    error = null;
    _retry = null;
    _expiresAt = null;
    _retryAt = null;
    selectedVendorId = null;
    identifier = '';
    _ticker?.cancel();
    _emit();
    _app?.auth.removeListener(_emit);
    _app?.dispose();
    _transport?.close();
    _transport = mode == VendorDataMode.mock
        ? MockVendorClient(
            scenario: scenario,
            onboardingScenario: onboardingScenario,
            dashboardScenario: dashboardScenario,
            latency: mockLatency,
          )
        : liveClientFactory?.call();
    _app = VendorApp(
      config: VendorAppConfig(
        origin: mode == VendorDataMode.mock
            ? 'https://mock.karryam.invalid'
            : liveOrigin,
      ),
      sessions: mode == VendorDataMode.mock
          ? MemoryVendorSessionStore()
          : liveSessionStore ??
                SecureVendorSessionStore(
                  namespace: Uri.parse(liveOrigin).origin,
                ),
      httpClient: _transport,
    );
    revision++;
    auth.addListener(_emit);
    try {
      await backend.initialize();
      if (_disposed) return;
      if (mode == VendorDataMode.live && auth.session != null) {
        await backend.client.refreshSession();
      }
      if (_disposed) return;
      final toured = await preferences.read('vendor.intro.completed') == 'true';
      if (_disposed) return;
      page = auth.session != null
          ? VendorPage.signedIn
          : toured
          ? VendorPage.signIn
          : VendorPage.onboarding;
    } catch (e) {
      if (_disposed) return;
      page = VendorPage.signIn;
      _handleError(e);
      _retry = _connect;
    } finally {
      _working = false;
      _emit();
    }
  }

  Future<void> changeMode(VendorDataMode next, {String? origin}) async {
    if (busy) return;
    if (next == VendorDataMode.live) {
      final value = (origin ?? liveOrigin).trim();
      VendorAppConfig(origin: value);
      await preferences.write('vendor.settings.origin', value);
      liveOrigin = value;
    }
    mode = next;
    await _connect();
  }

  Future<void> changeScenario(MockAuthScenario next) async {
    if (busy || mode != VendorDataMode.mock) return;
    scenario = next;
    await _connect();
  }

  Future<void> finishIntro() async {
    if (busy) return;
    page = VendorPage.signIn;
    _emit();
    try {
      await preferences.write('vendor.intro.completed', 'true');
    } catch (_) {}
  }

  void showIntro() {
    if (busy) return;
    error = null;
    page = VendorPage.onboarding;
    _emit();
  }

  void editContact() {
    if (busy) return;
    auth.sessionChanged(null);
    _expiresAt = null;
    _retry = null;
    error = null;
    page = VendorPage.signIn;
    _emit();
  }

  Map<String, dynamic>? registrationDraft;

  Future<void> start(
    String contact, {
    required bool email,
    Map<String, dynamic>? registration,
  }) async {
    if (!canRequest) return;
    registrationDraft = registration == null ? null : Map.of(registration);
    useEmail = email;
    identifier = email
        ? contact.trim()
        : contact.replaceAll(RegExp(r'[\s()-]'), '');
    await _perform(() async {
      final challenge = await auth.start(
        phone: email ? null : identifier,
        email: email ? identifier : null,
        registration: registration,
      );
      if (_disposed) return;
      _expiresAt = DateTime.now().add(Duration(seconds: challenge.expiresIn));
      _startTicker();
      page = VendorPage.code;
    });
  }

  Future<void> resend() async {
    if (!canRequest) return;
    await _perform(() async {
      final challenge = await auth.resend();
      if (_disposed) return;
      _expiresAt = DateTime.now().add(Duration(seconds: challenge.expiresIn));
      selectedVendorId = null;
      _startTicker();
      page = VendorPage.code;
    });
  }

  Future<void> verify(String code) async {
    if (!canRequest || codeExpired) return;
    await _perform(() async {
      await auth.verify(otp: code);
      if (_disposed) return;
      _afterVerification();
    });
  }

  void selectVendor(String id) {
    if (busy || !auth.availableVendorIds.contains(id)) return;
    selectedVendorId = id;
    _emit();
  }

  Future<void> chooseVendor() async {
    final id = selectedVendorId;
    if (!canRequest || id == null || codeExpired) return;
    await _perform(() async {
      await auth.chooseVendor(id);
      if (_disposed) return;
      _afterVerification();
    });
  }

  void _afterVerification() {
    if (auth.phase == VendorAuthPhase.vendorChoice) {
      selectedVendorId = auth.availableVendorIds.firstOrNull;
      page = VendorPage.vendorChoice;
    } else {
      _expiresAt = null;
      page = VendorPage.signedIn;
    }
  }

  Future<void> signOut() async {
    if (busy) return;
    await _perform(() async {
      try {
        await auth.logout();
      } finally {
        page = VendorPage.signIn;
        _expiresAt = null;
      }
    });
  }

  Future<void> retry() async {
    if (!canRequest) return;
    final action = _retry;
    if (action != null) {
      page = _retryPage;
      await _perform(action);
    }
  }

  Future<void> _perform(Future<void> Function() action) async {
    _working = true;
    error = null;
    _retry = null;
    _emit();
    try {
      await action();
    } catch (e) {
      if (_disposed) return;
      _retryPage = page;
      _handleError(e);
      if (e is ApiException && e.isRetryable) _retry = action;
    } finally {
      _working = false;
      _emit();
    }
  }

  void _handleError(Object value) {
    if (value is ApiException) {
      error = value.code == ApiErrorCode.network
          ? 'We couldn’t complete your request. Check your connection and try again.'
          : value.message;
      if (value.hasFieldErrors) {
        error = value.fields.map((f) => f.message).join('\n');
      }
      if (value.statusCode == 403) page = VendorPage.accessUnavailable;
      if (value.code == ApiErrorCode.network ||
          (value.statusCode ?? 0) >= 500) {
        _retryPage = page;
        page = VendorPage.connectionError;
      }
      final retryAfter = value.details['retry_after']?.toString();
      if (retryAfter != null) {
        _retryAt = _parseRetryAfter(retryAfter);
        _startTicker();
      }
      if (value.statusCode == 404 && page == VendorPage.code) {
        _expiresAt = null;
        page = VendorPage.signIn;
      }
    } else {
      error = 'We couldn’t complete your request. Please try again.';
    }
  }

  static DateTime? _parseRetryAfter(String value) {
    final seconds = int.tryParse(value);
    if (seconds != null) {
      return DateTime.now().add(Duration(seconds: seconds < 0 ? 0 : seconds));
    }
    final match = RegExp(
      r'\w+, (\d{2}) (\w{3}) (\d{4}) (\d{2}):(\d{2}):(\d{2}) GMT',
    ).firstMatch(value);
    if (match == null) return DateTime.tryParse(value);
    const months = [
      'Jan',
      'Feb',
      'Mar',
      'Apr',
      'May',
      'Jun',
      'Jul',
      'Aug',
      'Sep',
      'Oct',
      'Nov',
      'Dec',
    ];
    final month = months.indexOf(match[2]!) + 1;
    if (month == 0) return null;
    return DateTime.utc(
      int.parse(match[3]!),
      month,
      int.parse(match[1]!),
      int.parse(match[4]!),
      int.parse(match[5]!),
      int.parse(match[6]!),
    );
  }

  int _secondsUntil(DateTime? value) {
    if (value == null) return 0;
    final seconds = (value.difference(DateTime.now()).inMilliseconds / 1000)
        .ceil();
    return seconds > 0 ? seconds : 0;
  }

  void _startTicker() {
    _ticker?.cancel();
    _ticker = Timer.periodic(const Duration(seconds: 1), (_) {
      if (page == VendorPage.vendorChoice && codeExpired) {
        page = VendorPage.code;
      }
      _emit();
      if (remainingSeconds == 0 && retrySeconds == 0) _ticker?.cancel();
    });
  }

  void _emit() {
    if (!_disposed) notifyListeners();
  }

  @override
  void dispose() {
    _disposed = true;
    _ticker?.cancel();
    _app?.auth.removeListener(_emit);
    _app?.dispose();
    _transport?.close();
    super.dispose();
  }
}
