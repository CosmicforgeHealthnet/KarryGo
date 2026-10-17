import '../../../core/contracts/vendor_contract.dart';
import 'business_onboarding_controller.dart';

enum SetupState { notStarted, inProgress, underReview, complete, attention }

class SetupProgress {
  const SetupProgress(this.state, this.detail);
  final SetupState state;
  final String? detail;
  String get label => switch (state) {
    SetupState.notStarted => 'Not started',
    SetupState.inProgress => 'In progress',
    SetupState.underReview => 'Under review',
    SetupState.complete => 'Complete',
    SetupState.attention => 'Needs attention',
  };
}

String requirementStep(JsonObject r) => switch (r['code']) {
  'BUSINESS_BASICS' => 'business',
  'AUTHORIZED_REPRESENTATIVE' => 'representative',
  'FIRST_BRANCH' => 'locations',
  _ => switch (r['dimension']) {
    // Identity is verified from the documents step; counting it here left the
    // representative step looking unfinished after it was saved.
    'IDENTITY' => 'verification',
    'BANK' => 'bank',
    // Location is verified from the documents step, like identity.
    'LOCATION' => 'verification',
    _ =>
      ['CAPABILITY', 'BRANCH_CAPABILITY'].contains(r['scope'])
          ? 'capabilities'
          : 'verification',
  },
};

bool reviewPending(Object? status) => [
  'PENDING_REVIEW',
  'PENDING_VERIFICATION',
  'SUBMITTED',
  'IN_REVIEW',
].contains(status);
bool needsAttention(Object? status) => [
  'REJECTED',
  'EXPIRED',
  'SUSPENDED',
  'CLOSED',
  'ACTION_REQUIRED',
  'FAILED',
].contains(status);

SetupProgress stepProgress(BusinessOnboardingController c, String kind) {
  final requirements = c.requirements
      .where((r) => requirementStep(r) == kind)
      .toList();
  final records = switch (kind) {
    'business' => [c.business],
    'representative' => [c.representative],
    'capabilities' => c.capabilities,
    'locations' => c.branches.where((b) => b['status'] != 'CLOSED').toList(),
    'bank' => [c.bank],
    _ => c.documents,
  };
  final problem = requirements.where(
    (r) => r['satisfied'] != true && needsAttention(r['status']),
  );
  if (c.blocked ||
      problem.isNotEmpty ||
      records.any((r) => needsAttention(r['status']))) {
    return SetupProgress(
      SetupState.attention,
      problem.isEmpty
          ? null
          : problem.first['blocker']
                ?.toString()
                .replaceAll('_', ' ')
                .toLowerCase(),
    );
  }
  final present = switch (kind) {
    'business' => (c.business['legal_name']?.toString().isNotEmpty ?? false),
    'representative' => c.representative['authorized'] == true,
    'capabilities' => c.capabilities.isNotEmpty,
    'locations' => c.branches.any((b) => b['status'] != 'CLOSED'),
    'bank' => c.bank.isNotEmpty,
    _ =>
      c.documents.isNotEmpty ||
          requirements.any(
            (r) => r['satisfied'] == true || reviewPending(r['status']),
          ),
  };
  if (requirements.isNotEmpty &&
      requirements.every((r) => r['satisfied'] == true)) {
    return const SetupProgress(SetupState.complete, null);
  }
  if (requirements.any((r) => reviewPending(r['status'])) ||
      records.any((r) => reviewPending(r['status']))) {
    return const SetupProgress(SetupState.underReview, null);
  }
  if (requirements.isEmpty && present) {
    return SetupProgress(
      kind == 'verification' ? SetupState.inProgress : SetupState.complete,
      null,
    );
  }
  final missing = requirements.where((r) => r['satisfied'] != true);
  return SetupProgress(
    present ? SetupState.inProgress : SetupState.notStarted,
    present && missing.isNotEmpty
        ? missing.first['blocker']
              ?.toString()
              .replaceAll('_', ' ')
              .toLowerCase()
        : null,
  );
}

bool canFinishSetup(BusinessOnboardingController c) {
  if (!c.loaded || c.busy || c.error != null || c.blocked || c.activated) {
    return false;
  }
  if (c.readiness['ready'] == true) return true;
  if (c.submitted) return false;
  if (c.requirements.isEmpty || c.capabilities.isEmpty) {
    return false;
  }
  return c.requirements.every(
    (r) => r['satisfied'] == true || reviewPending(r['status']),
  );
}
