import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import 'package:flutter/foundation.dart';

import '../data/provider_auth_api.dart';
import '../data/provider_session_store.dart';
import '../models/provider_auth_models.dart';

enum ProviderAuthStatus {
  /// Restoring a saved session on launch.
  checking,
  phoneEntry,
  otpVerification,
  authenticated,
}

@immutable
class ProviderAuthState {
  const ProviderAuthState({
    required this.status,
    this.isLoading = false,
    this.error,
    this.phone = '',
    this.email = '',
    this.useEmail = false,
    this.challenge,
    this.session,
  });

  const ProviderAuthState.checking()
    : this(status: ProviderAuthStatus.checking);

  final ProviderAuthStatus status;
  final bool isLoading;
  final String? error;
  final String phone;
  final String email;

  /// Signing in with an email address rather than a phone number.
  final bool useEmail;

  /// Where the code was sent, for the code screen.
  String get destination => useEmail ? email : phone;
  final OtpChallenge? challenge;
  final ProviderSession? session;

  String? get accessToken => session?.accessToken;
  TaxiProvider? get provider => session?.provider;

  ProviderAuthState copyWith({
    ProviderAuthStatus? status,
    bool? isLoading,
    String? error,
    bool clearError = false,
    String? phone,
    String? email,
    bool? useEmail,
    OtpChallenge? challenge,
    ProviderSession? session,
    bool clearSession = false,
  }) {
    return ProviderAuthState(
      status: status ?? this.status,
      isLoading: isLoading ?? this.isLoading,
      error: clearError ? null : (error ?? this.error),
      phone: phone ?? this.phone,
      email: email ?? this.email,
      useEmail: useEmail ?? this.useEmail,
      challenge: challenge ?? this.challenge,
      session: clearSession ? null : (session ?? this.session),
    );
  }
}

/// Drives OTP sign-in (phone or email) and keeps the session fresh.
class ProviderAuthController extends ChangeNotifier {
  ProviderAuthController({
    required ProviderAuthApi api,
    required ProviderSessionStore sessionStore,
  }) : _api = api,
       _store = sessionStore;

  final ProviderAuthApi _api;
  final ProviderSessionStore _store;

  ProviderAuthState _state = const ProviderAuthState.checking();
  ProviderAuthState get state => _state;

  void _emit(ProviderAuthState next) {
    _state = next;
    notifyListeners();
  }

  /// Restores a saved session, refreshing it so a stale access token never
  /// reaches the first real request. A failed refresh is a clean sign-out,
  /// not an error screen.
  Future<void> initialize() async {
    final saved = await _store.read();
    if (saved == null || saved.refreshToken.isEmpty) {
      _emit(const ProviderAuthState(status: ProviderAuthStatus.phoneEntry));
      return;
    }
    try {
      final session = await _api.refresh(refreshToken: saved.refreshToken);
      await _store.write(session);
      _emit(ProviderAuthState(
        status: ProviderAuthStatus.authenticated,
        session: session,
      ));
    } on ApiException {
      await _store.clear();
      _emit(const ProviderAuthState(status: ProviderAuthStatus.phoneEntry));
    }
  }

  /// Switches the sign-in screen between phone and email.
  void setUseEmail(bool useEmail) =>
      _emit(_state.copyWith(useEmail: useEmail, clearError: true));

  Future<void> startAuth(String phone) async {
    final trimmed = phone.trim();
    if (trimmed.isEmpty) {
      _emit(_state.copyWith(error: 'Enter your phone number.'));
      return;
    }
    _emit(_state.copyWith(
      isLoading: true,
      clearError: true,
      phone: trimmed,
      useEmail: false,
    ));
    await _start(() => _api.startAuth(phone: trimmed));
  }

  /// Email sign-in: the code arrives by email.
  Future<void> startEmailAuth(String email) async {
    final trimmed = email.trim().toLowerCase();
    final at = trimmed.indexOf('@');
    if (at < 1 || at == trimmed.length - 1) {
      _emit(_state.copyWith(error: 'Enter a valid email address.'));
      return;
    }
    _emit(_state.copyWith(
      isLoading: true,
      clearError: true,
      email: trimmed,
      useEmail: true,
    ));
    await _start(() => _api.startAuth(email: trimmed));
  }

  Future<void> _start(Future<OtpChallenge> Function() start) async {
    try {
      final challenge = await start();
      _emit(_state.copyWith(
        status: ProviderAuthStatus.otpVerification,
        isLoading: false,
        challenge: challenge,
      ));
    } on ApiException catch (e) {
      _emit(_state.copyWith(isLoading: false, error: e.message));
    }
  }

  Future<void> verifyOtp(String otp) async {
    final challenge = _state.challenge;
    if (challenge == null) return;
    if (otp.trim().length < 4) {
      _emit(_state.copyWith(error: 'Enter the code we sent you.'));
      return;
    }
    _emit(_state.copyWith(isLoading: true, clearError: true));
    try {
      final session = await _api.verifyAuth(
        phone: _state.useEmail ? null : _state.phone,
        email: _state.useEmail ? _state.email : null,
        challengeId: challenge.challengeId,
        otp: otp.trim(),
      );
      await _store.write(session);
      _emit(ProviderAuthState(
        status: ProviderAuthStatus.authenticated,
        session: session,
      ));
    } on ApiException catch (e) {
      _emit(_state.copyWith(isLoading: false, error: e.message));
    }
  }

  void backToPhoneEntry() {
    _emit(_state.copyWith(
      status: ProviderAuthStatus.phoneEntry,
      clearError: true,
    ));
  }

  /// Reflects a profile change (e.g. the parcel toggle) back into the session
  /// so every screen reading it stays in step without a re-login.
  void applyProviderUpdate(TaxiProvider provider) {
    final session = _state.session;
    if (session == null) return;
    final next = session.withProvider(provider);
    _emit(_state.copyWith(session: next));
    unawaitedWrite(next);
  }

  void unawaitedWrite(ProviderSession session) {
    // Fire-and-forget: a failed persist only costs a re-login next launch.
    _store.write(session);
  }

  Future<void> logout() async {
    final session = _state.session;
    if (session != null && session.refreshToken.isNotEmpty) {
      try {
        await _api.logout(refreshToken: session.refreshToken);
      } on ApiException {
        // Server-side revocation failed; the local session still goes.
      }
    }
    await _store.clear();
    _emit(const ProviderAuthState(status: ProviderAuthStatus.phoneEntry));
  }

  /// Called when any authenticated request comes back 401: the session is
  /// gone server-side, so drop it here rather than leaving the driver on a
  /// screen whose every action will fail.
  Future<void> handleAuthFailure() async {
    if (_state.status != ProviderAuthStatus.authenticated) return;
    await _store.clear();
    _emit(const ProviderAuthState(
      status: ProviderAuthStatus.phoneEntry,
      error: 'Your session expired. Please sign in again.',
    ));
  }
}
