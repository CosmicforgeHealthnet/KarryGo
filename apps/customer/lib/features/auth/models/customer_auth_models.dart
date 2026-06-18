import 'dart:convert';

class CustomerProfile {
  const CustomerProfile({
    required this.id,
    required this.phone,
    this.email = '',
    required this.onboardingStatus,
    this.firstName,
    this.lastName,
    this.status,
  });

  final String id;
  final String phone;
  final String email;
  final String onboardingStatus;
  final String? firstName;
  final String? lastName;
  final String? status;

  bool get requiresProfile => onboardingStatus == 'profile_required';

  String get displayName {
    final parts = [firstName, lastName]
        .where((part) => part != null && part.trim().isNotEmpty)
        .cast<String>()
        .toList();
    if (parts.isEmpty) {
      if (email.trim().isNotEmpty) {
        return email;
      }
      return phone;
    }
    return parts.join(' ');
  }

  factory CustomerProfile.fromJson(Map<String, dynamic> json) {
    return CustomerProfile(
      id: json['id']?.toString() ?? '',
      phone: json['phone']?.toString() ?? '',
      email: json['email']?.toString() ?? '',
      firstName: json['first_name']?.toString(),
      lastName: json['last_name']?.toString(),
      onboardingStatus:
          json['onboarding_status']?.toString() ?? 'profile_required',
      status: json['status']?.toString(),
    );
  }

  Map<String, dynamic> toJson() {
    return {
      'id': id,
      'phone': phone,
      'email': email,
      'first_name': firstName,
      'last_name': lastName,
      'onboarding_status': onboardingStatus,
      'status': status,
    };
  }
}

class StartAuthResult {
  const StartAuthResult({
    required this.challengeId,
    required this.expiresIn,
    this.debugOtp,
  });

  final String challengeId;
  final int expiresIn;
  final String? debugOtp;

  factory StartAuthResult.fromJson(Map<String, dynamic> json) {
    return StartAuthResult(
      challengeId: json['challenge_id']?.toString() ?? '',
      expiresIn: _intFromJson(json['expires_in']),
      debugOtp: json['debug_otp']?.toString(),
    );
  }
}

class AuthTokenResult {
  const AuthTokenResult({
    required this.accessToken,
    required this.refreshToken,
    required this.expiresIn,
    required this.customer,
  });

  final String accessToken;
  final String refreshToken;
  final int expiresIn;
  final CustomerProfile customer;

  factory AuthTokenResult.fromJson(Map<String, dynamic> json) {
    return AuthTokenResult(
      accessToken: json['access_token']?.toString() ?? '',
      refreshToken: json['refresh_token']?.toString() ?? '',
      expiresIn: _intFromJson(json['expires_in']),
      customer: CustomerProfile.fromJson(
        Map<String, dynamic>.from(json['customer'] as Map),
      ),
    );
  }

  CustomerSession toSession(DateTime now) {
    return CustomerSession(
      accessToken: accessToken,
      refreshToken: refreshToken,
      expiresAt: now.add(Duration(seconds: expiresIn)),
      customer: customer,
    );
  }
}

class CustomerSession {
  const CustomerSession({
    required this.accessToken,
    required this.refreshToken,
    required this.expiresAt,
    required this.customer,
  });

  final String accessToken;
  final String refreshToken;
  final DateTime expiresAt;
  final CustomerProfile customer;

  bool get isAccessTokenExpired {
    return DateTime.now().isAfter(
      expiresAt.subtract(const Duration(seconds: 30)),
    );
  }

  CustomerSession copyWith({
    String? accessToken,
    String? refreshToken,
    DateTime? expiresAt,
    CustomerProfile? customer,
  }) {
    return CustomerSession(
      accessToken: accessToken ?? this.accessToken,
      refreshToken: refreshToken ?? this.refreshToken,
      expiresAt: expiresAt ?? this.expiresAt,
      customer: customer ?? this.customer,
    );
  }

  factory CustomerSession.fromJson(Map<String, dynamic> json) {
    return CustomerSession(
      accessToken: json['access_token']?.toString() ?? '',
      refreshToken: json['refresh_token']?.toString() ?? '',
      expiresAt:
          DateTime.tryParse(json['expires_at']?.toString() ?? '') ??
          DateTime.fromMillisecondsSinceEpoch(0),
      customer: CustomerProfile.fromJson(
        Map<String, dynamic>.from(json['customer'] as Map),
      ),
    );
  }

  factory CustomerSession.fromJsonString(String value) {
    final decoded = jsonDecode(value);
    return CustomerSession.fromJson(Map<String, dynamic>.from(decoded as Map));
  }

  Map<String, dynamic> toJson() {
    return {
      'access_token': accessToken,
      'refresh_token': refreshToken,
      'expires_at': expiresAt.toIso8601String(),
      'customer': customer.toJson(),
    };
  }

  String toJsonString() => jsonEncode(toJson());
}

int _intFromJson(Object? value) {
  if (value is int) {
    return value;
  }
  if (value is num) {
    return value.toInt();
  }
  return int.tryParse(value?.toString() ?? '') ?? 0;
}
