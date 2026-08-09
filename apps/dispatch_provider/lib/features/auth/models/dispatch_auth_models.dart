class AuthStartResponse {
  final String message;
  final int expiresInSeconds;

  /// Only present when the backend has DISPATCH_RIDER_DEBUG_OTP=true (dev/local
  /// only) — never present against a production backend.
  final String? debugOtp;

  AuthStartResponse({
    required this.message,
    required this.expiresInSeconds,
    this.debugOtp,
  });

  factory AuthStartResponse.fromJson(Map<String, dynamic> json) {
    return AuthStartResponse(
      message: json['message'] as String? ?? '',
      expiresInSeconds: (json['expires_in_seconds'] as num?)?.toInt() ?? 0,
      debugOtp: json['debug_otp'] as String?,
    );
  }
}

class AuthVerifyResponse {
  final String providerId;
  final String role;
  final String tokenType;
  final String accessToken;
  final String refreshToken;
  final int expiresInSeconds;
  final ProviderMode providerMode;
  final ProviderOrganizationSummary? organization;
  final ProviderMembershipSummary? membership;
  final ProviderAssignmentPolicy? assignmentPolicy;
  final String? fieldVisibilityPolicy;

  AuthVerifyResponse({
    required this.providerId,
    required this.role,
    required this.tokenType,
    required this.accessToken,
    required this.refreshToken,
    required this.expiresInSeconds,
    this.providerMode = ProviderMode.independent,
    this.organization,
    this.membership,
    this.assignmentPolicy,
    this.fieldVisibilityPolicy,
  });

  factory AuthVerifyResponse.fromJson(Map<String, dynamic> json) {
    return AuthVerifyResponse(
      providerId: json['provider_id'] as String? ?? '',
      role: json['role'] as String? ?? '',
      tokenType: json['token_type'] as String? ?? 'Bearer',
      accessToken: json['access_token'] as String? ?? '',
      refreshToken: json['refresh_token'] as String? ?? '',
      expiresInSeconds: (json['expires_in_seconds'] as num?)?.toInt() ?? 0,
      providerMode: ProviderMode.fromWire(json['providerMode'] as String?),
      organization: json['organization'] is Map<String, dynamic>
          ? ProviderOrganizationSummary.fromJson(
              json['organization'] as Map<String, dynamic>,
            )
          : null,
      membership: json['membership'] is Map<String, dynamic>
          ? ProviderMembershipSummary.fromJson(
              json['membership'] as Map<String, dynamic>,
            )
          : null,
      assignmentPolicy: json['assignmentPolicy'] is Map<String, dynamic>
          ? ProviderAssignmentPolicy.fromJson(
              json['assignmentPolicy'] as Map<String, dynamic>,
            )
          : null,
      fieldVisibilityPolicy: json['fieldVisibilityPolicy'] as String?,
    );
  }
}

class AuthRefreshResponse {
  final String accessToken;
  final String refreshToken;
  final String tokenType;
  final int expiresInSeconds;
  final ProviderMode providerMode;
  final ProviderOrganizationSummary? organization;
  final ProviderMembershipSummary? membership;
  final ProviderAssignmentPolicy? assignmentPolicy;
  final String? fieldVisibilityPolicy;

  AuthRefreshResponse({
    required this.accessToken,
    required this.refreshToken,
    required this.tokenType,
    required this.expiresInSeconds,
    this.providerMode = ProviderMode.independent,
    this.organization,
    this.membership,
    this.assignmentPolicy,
    this.fieldVisibilityPolicy,
  });

  factory AuthRefreshResponse.fromJson(Map<String, dynamic> json) {
    return AuthRefreshResponse(
      accessToken: json['access_token'] as String? ?? '',
      refreshToken: json['refresh_token'] as String? ?? '',
      tokenType: json['token_type'] as String? ?? 'Bearer',
      expiresInSeconds: (json['expires_in_seconds'] as num?)?.toInt() ?? 0,
      providerMode: ProviderMode.fromWire(json['providerMode'] as String?),
      organization: json['organization'] is Map<String, dynamic>
          ? ProviderOrganizationSummary.fromJson(
              json['organization'] as Map<String, dynamic>,
            )
          : null,
      membership: json['membership'] is Map<String, dynamic>
          ? ProviderMembershipSummary.fromJson(
              json['membership'] as Map<String, dynamic>,
            )
          : null,
      assignmentPolicy: json['assignmentPolicy'] is Map<String, dynamic>
          ? ProviderAssignmentPolicy.fromJson(
              json['assignmentPolicy'] as Map<String, dynamic>,
            )
          : null,
      fieldVisibilityPolicy: json['fieldVisibilityPolicy'] as String?,
    );
  }
}

enum ProviderMode {
  independent,
  organizationManaged;

  static ProviderMode fromWire(String? value) => value == 'ORGANIZATION_MANAGED'
      ? ProviderMode.organizationManaged
      : ProviderMode.independent;
}

class ProviderOrganizationSummary {
  const ProviderOrganizationSummary({
    required this.id,
    required this.displayName,
    required this.status,
    this.branch,
  });
  final String id;
  final String displayName;
  final String status;
  final ProviderBranchSummary? branch;

  factory ProviderOrganizationSummary.fromJson(Map<String, dynamic> json) =>
      ProviderOrganizationSummary(
        id: json['id'] as String? ?? '',
        displayName: json['displayName'] as String? ?? '',
        status: json['status'] as String? ?? '',
        branch: json['branch'] is Map<String, dynamic>
            ? ProviderBranchSummary.fromJson(
                json['branch'] as Map<String, dynamic>,
              )
            : null,
      );
}

class ProviderBranchSummary {
  const ProviderBranchSummary({
    required this.id,
    required this.name,
    required this.type,
  });
  final String id;
  final String name;
  final String type;

  factory ProviderBranchSummary.fromJson(Map<String, dynamic> json) =>
      ProviderBranchSummary(
        id: json['id'] as String? ?? '',
        name: json['name'] as String? ?? '',
        type: json['type'] as String? ?? '',
      );
}

class ProviderMembershipSummary {
  const ProviderMembershipSummary({
    required this.status,
    required this.role,
    required this.serviceCategory,
  });
  final String status;
  final String role;
  final String serviceCategory;

  factory ProviderMembershipSummary.fromJson(Map<String, dynamic> json) =>
      ProviderMembershipSummary(
        status: json['status'] as String? ?? '',
        role: json['role'] as String? ?? '',
        serviceCategory: json['serviceCategory'] as String? ?? '',
      );
}

class ProviderAssignmentPolicy {
  const ProviderAssignmentPolicy({
    required this.assignmentMode,
    required this.providerDeclineRequired,
  });
  final String assignmentMode;
  final bool providerDeclineRequired;

  factory ProviderAssignmentPolicy.fromJson(Map<String, dynamic> json) =>
      ProviderAssignmentPolicy(
        assignmentMode: json['assignmentMode'] as String? ?? '',
        providerDeclineRequired:
            json['providerDeclineRequired'] as bool? ?? true,
      );
}
