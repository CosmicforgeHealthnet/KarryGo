// Shared date wording, so every screen says dates the same way.

const _months = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec'];
const _weekdays = ['Monday', 'Tuesday', 'Wednesday', 'Thursday', 'Friday', 'Saturday', 'Sunday'];

/// "Monday, 22 May 2026".
String formatLongDate(DateTime dt) {
  final d = dt.toLocal();
  return '${_weekdays[d.weekday - 1]}, ${d.day} ${_months[d.month - 1]} ${d.year}';
}

/// "22 May 2026".
String formatShortDate(DateTime dt) {
  final d = dt.toLocal();
  return '${d.day} ${_months[d.month - 1]} ${d.year}';
}

/// "5:21 PM".
String formatClock(DateTime dt) {
  final d = dt.toLocal();
  final h = d.hour % 12 == 0 ? 12 : d.hour % 12;
  final m = d.minute.toString().padLeft(2, '0');
  return '$h:$m ${d.hour < 12 ? 'AM' : 'PM'}';
}

/// "22 May 2026 · 5:21 PM".
String formatDateTime(DateTime dt) => '${formatShortDate(dt)} · ${formatClock(dt)}';

/// "Today", "Yesterday", "Monday" within a week, else "22 May 2026".
String formatRelativeDay(DateTime dt, {DateTime? now}) {
  final d = dt.toLocal();
  final n = (now ?? DateTime.now()).toLocal();
  final days = DateTime(n.year, n.month, n.day).difference(DateTime(d.year, d.month, d.day)).inDays;
  if (days == 0) return 'Today';
  if (days == 1) return 'Yesterday';
  if (days > 1 && days < 7) return _weekdays[d.weekday - 1];
  return formatShortDate(d);
}
