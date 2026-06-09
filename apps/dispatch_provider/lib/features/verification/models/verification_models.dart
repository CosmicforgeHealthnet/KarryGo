class VerificationStepSummary {
  final String step;
  final String status;
  final bool isOptional;
  final DateTime? submittedAt;
  final DateTime? reviewedAt;

  const VerificationStepSummary({
    required this.step,
    required this.status,
    required this.isOptional,
    this.submittedAt,
    this.reviewedAt,
  });

  factory VerificationStepSummary.fromJson(Map<String, dynamic> json) {
    return VerificationStepSummary(
      step: json['step'] as String? ?? '',
      status: json['status'] as String? ?? 'pending',
      isOptional: json['is_optional'] as bool? ?? false,
      submittedAt: json['submitted_at'] != null
          ? DateTime.tryParse(json['submitted_at'] as String)
          : null,
      reviewedAt: json['reviewed_at'] != null
          ? DateTime.tryParse(json['reviewed_at'] as String)
          : null,
    );
  }
}

class AllStatusResponse {
  final String overallStatus;
  final int completionPercentage;
  final List<VerificationStepSummary> steps;

  const AllStatusResponse({
    required this.overallStatus,
    required this.completionPercentage,
    required this.steps,
  });

  factory AllStatusResponse.fromJson(Map<String, dynamic> json) {
    final rawSteps = json['steps'] as List?;
    final stepsList = rawSteps != null
        ? rawSteps
            .map((s) => VerificationStepSummary.fromJson(Map<String, dynamic>.from(s as Map)))
            .toList()
        : const <VerificationStepSummary>[];

    return AllStatusResponse(
      overallStatus: json['overall_status'] as String? ?? 'not_started',
      completionPercentage: (json['completion_percentage'] as num?)?.toInt() ?? 0,
      steps: stepsList,
    );
  }
}
