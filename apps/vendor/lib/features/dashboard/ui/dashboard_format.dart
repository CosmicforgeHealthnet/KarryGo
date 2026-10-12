String dashboardLabel(Object? value) => (value?.toString() ?? '')
    .toLowerCase()
    .split('_')
    .map(
      (word) =>
          word.isEmpty ? word : '${word[0].toUpperCase()}${word.substring(1)}',
    )
    .join(' ');
String dashboardMoney(Object? minor, Object? currency) {
  if (minor is! int || currency is! String) return 'Unavailable';
  final absolute = minor.abs();
  final whole = (absolute ~/ 100).toString().replaceAllMapped(
    RegExp(r'(\d)(?=(\d{3})+(?!\d))'),
    (match) => '${match[1]},',
  );
  return '${minor < 0 ? '-' : ''}$currency $whole.${(absolute % 100).toString().padLeft(2, '0')}';
}
