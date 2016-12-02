function normalize() {
  var n = []
  for (var i=0; i<arguments.length; i++) {
    if (arguments[i]) {
      n.push(arguments[i].toLowerCase().replace(/[^a-z0-9]/g, ''))
    }
  }
  return n.join('-').substr(0, 24)
}

$(document).ready(function () {
  $("#region_selector").change(function (e) {
    window.location.href = e.target.value;
  });
});