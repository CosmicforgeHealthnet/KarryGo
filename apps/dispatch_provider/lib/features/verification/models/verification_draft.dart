import 'package:flutter/foundation.dart';

/// In-memory draft of everything the user typed across the verification
/// flow — including fields the backend doesn't accept yet.
/// On LIVE these extra fields are silently dropped in the API call.
/// On MOCK they're kept in [VerificationMock] so the UI stays fully populated.
@immutable
class VerificationDraft {
  // ── Identity ──
  final String? fullName;
  final DateTime? dateOfBirth;
  final String? gender; // 'male' | 'female' | null
  final String? govtIdType; // 'nin' | 'voter_card' | 'passport' | 'drivers_licence'
  final String? govtIdNumber;
  final String? govtIdFilePath;
  final String? profilePhotoFilePath;

  // ── Licence ──
  final String? licenceNumber;
  final String? licenceType; // 'private_b' | 'commercial' | null
  final String? issuingState; // e.g. 'Lagos'
  final String? expiryYear;
  final String? expiryMonth;
  final String? licenceFilePath;

  // ── Face ──
  final String? selfieFilePath;

  const VerificationDraft({
    this.fullName,
    this.dateOfBirth,
    this.gender,
    this.govtIdType,
    this.govtIdNumber,
    this.govtIdFilePath,
    this.profilePhotoFilePath,
    this.licenceNumber,
    this.licenceType,
    this.issuingState,
    this.expiryYear,
    this.expiryMonth,
    this.licenceFilePath,
    this.selfieFilePath,
  });

  VerificationDraft copyWith({
    String? fullName,
    DateTime? dateOfBirth,
    String? gender,
    String? govtIdType,
    String? govtIdNumber,
    String? govtIdFilePath,
    String? profilePhotoFilePath,
    String? licenceNumber,
    String? licenceType,
    String? issuingState,
    String? expiryYear,
    String? expiryMonth,
    String? licenceFilePath,
    String? selfieFilePath,
  }) {
    return VerificationDraft(
      fullName: fullName ?? this.fullName,
      dateOfBirth: dateOfBirth ?? this.dateOfBirth,
      gender: gender ?? this.gender,
      govtIdType: govtIdType ?? this.govtIdType,
      govtIdNumber: govtIdNumber ?? this.govtIdNumber,
      govtIdFilePath: govtIdFilePath ?? this.govtIdFilePath,
      profilePhotoFilePath: profilePhotoFilePath ?? this.profilePhotoFilePath,
      licenceNumber: licenceNumber ?? this.licenceNumber,
      licenceType: licenceType ?? this.licenceType,
      issuingState: issuingState ?? this.issuingState,
      expiryYear: expiryYear ?? this.expiryYear,
      expiryMonth: expiryMonth ?? this.expiryMonth,
      licenceFilePath: licenceFilePath ?? this.licenceFilePath,
      selfieFilePath: selfieFilePath ?? this.selfieFilePath,
    );
  }
}