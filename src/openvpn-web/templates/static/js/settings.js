$(document).on('click', '#settings', function () {
  request.get('/settings').then((data) => {
    $('#siteUrl').val(data.system.base.site_url);
    $('#webPort').val(data.system.base.web_port);
    $('#adminUsername').val(data.system.base.admin_username);
    $('#adminPassword').val(data.system.base.admin_password);
    $('#historyMaxDays').val(data.system.base.history_max_days);
    $('#autoUpdateOvpnConfig').prop('checked', data.system.base.auto_update_ovpn_config);
    $('#allowDuplicateLogin').prop('checked', data.system.base.allow_duplicate_login);

    $('#ldapAuth').prop('checked', data.system.ldap.ldap_auth);
    $('#ldapUrl').val(data.system.ldap.ldap_url);
    $('#ldapBindDn').val(data.system.ldap.ldap_bind_user_dn);
    $('#ldapBindPassword').val(data.system.ldap.ldap_bind_password);
    $('#ldapBaseDn').val(data.system.ldap.ldap_base_dn);
    $('#ldapUserAttribute').val(data.system.ldap.ldap_user_attribute);
    $('#ldapUserAttrIpaddrName').val(data.system.ldap.ldap_user_attr_ipaddr_name);
    $('#ldapUserAttrConfigName').val(data.system.ldap.ldap_user_attr_config_name);
    $('#ldapUserGroupFilter').prop('checked', data.system.ldap.ldap_user_group_filter);
    $('#ldapUserGroupDn').val(data.system.ldap.ldap_user_group_dn);

    $('#emailSendSubjectPrefix').val(data.system.email?.send_subject_prefix);
    $('#emailSendFrom').val(data.system.email?.send_from);
    $('#emailSmtpHost').val(data.system.email?.host);
    $('#emailSmtpPort').val(data.system.email?.port);
    $('#emailSmtpUsername').val(data.system.email?.username);
    $('#emailSmtpPassword').val(data.system.email?.password);
    $('#emailSecurityTls').prop('checked', data.system.email?.security === 'tls');
    $('#emailSecuritySsl').prop('checked', data.system.email?.security === 'ssl');

    $('#windowsClientUrl').val(data.client.client_url.windows);
    $('#macosClientUrl').val(data.client.client_url.macos);
    $('#linuxClientUrl').val(data.client.client_url.linux);
    $('#androidClientUrl').val(data.client.client_url.android);
    $('#iosClientUrl').val(data.client.client_url.ios);

    $('#ovpnPort').val(data.openvpn.ovpn_port);
    $('#ovpnProto').val(data.openvpn.ovpn_proto);
    $('#ovpnGateway').prop('checked', data.openvpn.ovpn_gateway);
    $('#ovpnMaxClients').val(data.openvpn.ovpn_max_clients);
    $('#ovpnSubnet').val(data.openvpn.ovpn_subnet);
    $('#ovpnManagement').val(data.openvpn.ovpn_management);
    $('#ovpnIpv6').prop('checked', data.openvpn.ovpn_ipv6);
    $('#ovpnSubnet6').val(data.openvpn.ovpn_subnet6);
    $('#ovpnPushDns1').val(data.openvpn.ovpn_push_dns1);
    $('#ovpnPushDns2').val(data.openvpn.ovpn_push_dns2);

    $('#settingsModal').modal('show');
  });
});

// base
$(document).on('focus', '#siteUrl', function () {
  $(this).data('oldValue', $(this).val());
});

$(document).on('blur', '#siteUrl', function () {
  const oldValue = $(this).data('oldValue');
  const newValue = $(this).val();

  if (oldValue === newValue) return;

  request.post('/settings', { 'system.base.site_url': newValue }).then((data) => {
    message.success(data.message);
  });
});

$(document).on('focus', '#webPort', function () {
  $(this).data('oldValue', $(this).val());
});

$(document).on('blur', '#webPort', function () {
  const oldValue = $(this).data('oldValue');
  const newValue = $(this).val();

  if (oldValue === newValue) return;

  request.post('/settings', { 'system.base.web_port': newValue }).then((data) => {
    message.success(data.message);
  });
});

$(document).on('focus', '#adminUsername', function () {
  $(this).data('oldValue', $(this).val());
});

$(document).on('blur', '#adminUsername', function () {
  const oldValue = $(this).data('oldValue');
  const newValue = $(this).val();

  if (oldValue === newValue) return;

  if (confirm('修改用户名将会重新登录，确认吗？')) {
    request.post('/settings', { 'system.base.admin_username': newValue }).then((data) => {
      message.success(data.message);
      window.location.replace('/logout');
    });
  } else {
    $(this).val(oldValue);
  }
});

function checkPassword(pw) {
  let passed = 0;
  const rules = {
    length: (s) => s.length >= 12,
    lower: (s) => /[a-z]/.test(s),
    upper: (s) => /[A-Z]/.test(s),
    digit: (s) => /[0-9]/.test(s),
    special: (s) => /[!@#\$%\^&\*()_+\-=[\]{};':"\\|,.<>\/?]/.test(s),
  };

  for (const key in rules) {
    if (rules[key](pw)) passed++;
  }
  const percent = Math.round((passed / Object.keys(rules).length) * 100);

  return percent;
}

$(document).on('input', '#adminPassword', function () {
  const percent = checkPassword($(this).val());
  const progressBar = $('#settingsModal .progress-bar');

  progressBar.width(percent + '%');
  progressBar.attr(
    'class',
    percent < 60
      ? 'progress-bar text-bg-danger'
      : 100 < percent > 60
      ? 'progress-bar bg-warning'
      : percent == 100
      ? 'progress-bar bg-success'
      : 'progress-bar bg-warning'
  );

  $(this).data('passwordScore', percent);
});

$(document).on('focus', '#adminPassword', function () {
  $(this).val('');
});

$(document).on('blur', '#adminPassword', function () {
  const passwordScore = $(this).data('passwordScore');
  const newValue = $(this).val();

  if (passwordScore != 100) {
    message.error('密码不满足要求（长度12位，包含大小写字母、数字、特殊字符）');
    $('#settingsModal .progress-bar').width('0%');
    return;
  }

  request.post('/settings', { 'system.base.admin_password': newValue }).then((data) => {
    message.success(data.message);
    $('#settingsModal .progress-bar').width('0%');
  });
});

$(document).on('focus', '#historyMaxDays', function () {
  $(this).data('oldValue', $(this).val());
});

$(document).on('blur', '#historyMaxDays', function () {
  const oldValue = $(this).data('oldValue');
  const newValue = $(this).val();

  if (oldValue === newValue) return;

  request.post('/settings', { 'system.base.history_max_days': newValue }).then((data) => {
    message.success(data.message);
  });
});

$(document).on('change', '#autoUpdateOvpnConfig', function () {
  request.post('/settings', { 'system.base.auto_update_ovpn_config': $(this).prop('checked') }).then((data) => {
    message.success(data.message);
  });
});

$(document).on('change', '#allowDuplicateLogin', function () {
  request.post('/settings', { 'system.base.allow_duplicate_login': $(this).prop('checked') }).then((data) => {
    message.success(data.message);
  });
});

// ldap
$(document).on('change', '#ldapAuth', function () {
  request.post('/settings', { 'system.ldap.ldap_auth': $(this).prop('checked') }).then((data) => {
    message.success(data.message);
  });
});

$(document).on('focus', '#ldapUrl', function () {
  $(this).data('oldValue', $(this).val());
});

$(document).on('blur', '#ldapUrl', function () {
  const oldValue = $(this).data('oldValue');
  const newValue = $(this).val();

  if (oldValue === newValue) return;

  request.post('/settings', { 'system.ldap.ldap_url': newValue }).then((data) => {
    message.success(data.message);
  });
});

$(document).on('focus', '#ldapBindDn', function () {
  $(this).data('oldValue', $(this).val());
});

$(document).on('blur', '#ldapBindDn', function () {
  const oldValue = $(this).data('oldValue');
  const newValue = $(this).val();

  if (oldValue === newValue) return;

  request.post('/settings', { 'system.ldap.ldap_bind_user_dn': newValue }).then((data) => {
    message.success(data.message);
  });
});

$(document).on('focus', '#ldapBindPassword', function () {
  $(this).data('oldValue', $(this).val());
});

$(document).on('blur', '#ldapBindPassword', function () {
  const oldValue = $(this).data('oldValue');
  const newValue = $(this).val();

  if (oldValue === newValue) return;

  request.post('/settings', { 'system.ldap.ldap_bind_password': newValue }).then((data) => {
    message.success(data.message);
  });
});

$(document).on('focus', '#ldapBaseDn', function () {
  $(this).data('oldValue', $(this).val());
});

$(document).on('blur', '#ldapBaseDn', function () {
  const oldValue = $(this).data('oldValue');
  const newValue = $(this).val();

  if (oldValue === newValue) return;

  request.post('/settings', { 'system.ldap.ldap_base_dn': newValue }).then((data) => {
    message.success(data.message);
  });
});

$(document).on('focus', '#ldapUserAttribute', function () {
  $(this).data('oldValue', $(this).val());
});

$(document).on('blur', '#ldapUserAttribute', function () {
  const oldValue = $(this).data('oldValue');
  const newValue = $(this).val();

  if (oldValue === newValue) return;

  request.post('/settings', { 'system.ldap.ldap_user_attribute': newValue }).then((data) => {
    message.success(data.message);
  });
});

$(document).on('focus', '#ldapUserAttrIpaddrName', function () {
  $(this).data('oldValue', $(this).val());
});

$(document).on('blur', '#ldapUserAttrIpaddrName', function () {
  const oldValue = $(this).data('oldValue');
  const newValue = $(this).val();

  if (oldValue === newValue) return;

  request.post('/settings', { 'system.ldap.ldap_user_attr_ipaddr_name': newValue }).then((data) => {
    message.success(data.message);
  });
});

$(document).on('focus', '#ldapUserAttrConfigName', function () {
  $(this).data('oldValue', $(this).val());
});

$(document).on('blur', '#ldapUserAttrConfigName', function () {
  const oldValue = $(this).data('oldValue');
  const newValue = $(this).val();

  if (oldValue === newValue) return;

  request.post('/settings', { 'system.ldap.ldap_user_attr_config_name': newValue }).then((data) => {
    message.success(data.message);
  });
});

$(document).on('change', '#ldapUserGroupFilter', function () {
  request.post('/settings', { 'system.ldap.ldap_user_group_filter': $(this).prop('checked') }).then((data) => {
    message.success(data.message);
  });
});

$(document).on('focus', '#ldapUserGroupDn', function () {
  $(this).data('oldValue', $(this).val());
});

$(document).on('blur', '#ldapUserGroupDn', function () {
  const oldValue = $(this).data('oldValue');
  const newValue = $(this).val();

  if (oldValue === newValue) return;

  request.post('/settings', { 'system.ldap.ldap_user_group_dn': newValue }).then((data) => {
    message.success(data.message);
  });
});

// email
$(document).on('mousedown', '#emailSendSubjectPrefix', function () {
  if (!$(this).is(':focus')) {
    $(this).data('oldValue', $(this).val());
  }
});

$(document).on('blur', '#emailSendSubjectPrefix', function () {
  const oldValue = $(this).data('oldValue');
  const newValue = $(this).val();

  if (oldValue === newValue) return;

  request.post('/settings', { 'system.email.send_subject_prefix': newValue }).then((data) => {
    message.success(data.message);
  });
});

$(document).on('mousedown', '#emailSendFrom', function () {
  if (!$(this).is(':focus')) {
    $(this).data('oldValue', $(this).val());
  }
});

$(document).on('blur', '#emailSendFrom', function () {
  if (!this.reportValidity()) return;

  const oldValue = $(this).data('oldValue');
  const newValue = $(this).val();

  if (oldValue === newValue) return;

  request.post('/settings', { 'system.email.send_from': newValue }).then((data) => {
    message.success(data.message);
  });
});

$(document).on('mousedown', '#emailSmtpHost', function () {
  if (!$(this).is(':focus')) {
    $(this).data('oldValue', $(this).val());
  }
});

$(document).on('blur', '#emailSmtpHost', function () {
  const oldValue = $(this).data('oldValue');
  const newValue = $(this).val();

  if (oldValue === newValue) return;

  request.post('/settings', { 'system.email.host': newValue }).then((data) => {
    message.success(data.message);
  });
});

$(document).on('mousedown', '#emailSmtpPort', function () {
  if (!$(this).is(':focus')) {
    $(this).data('oldValue', $(this).val());
  }
});

$(document).on('blur', '#emailSmtpPort', function () {
  const oldValue = $(this).data('oldValue');
  const newValue = $(this).val();

  if (oldValue === newValue) return;

  request.post('/settings', { 'system.email.port': newValue }).then((data) => {
    message.success(data.message);
  });
});

$(document).on('mousedown', '#emailSmtpUsername', function () {
  if (!$(this).is(':focus')) {
    $(this).data('oldValue', $(this).val());
  }
});

$(document).on('blur', '#emailSmtpUsername', function () {
  if (!this.reportValidity()) return;

  const oldValue = $(this).data('oldValue');
  const newValue = $(this).val();

  if (oldValue === newValue) return;

  request.post('/settings', { 'system.email.username': newValue }).then((data) => {
    message.success(data.message);
  });
});

$(document).on('mousedown', '#emailSmtpPassword', function () {
  if (!$(this).is(':focus')) {
    $(this).data('oldValue', $(this).val());
  }
});

$(document).on('blur', '#emailSmtpPassword', function () {
  const oldValue = $(this).data('oldValue');
  const newValue = $(this).val();

  if (oldValue === newValue) return;

  request.post('/settings', { 'system.email.password': newValue }).then((data) => {
    message.success(data.message);
  });
});

$(document).on('change', '#emailSecurityTls', function () {
  if ($('#emailSecuritySsl').prop('checked')) {
    $('#emailSecuritySsl').prop('checked', false);
  }

  request.post('/settings', { 'system.email.security': $(this).prop('checked') ? 'tls' : '' }).then((data) => {
    message.success(data.message);
  });
});

$(document).on('change', '#emailSecuritySsl', function () {
  if ($('#emailSecurityTls').prop('checked')) {
    $('#emailSecurityTls').prop('checked', false);
  }

  request.post('/settings', { 'system.email.security': $(this).prop('checked') ? 'ssl' : '' }).then((data) => {
    message.success(data.message);
  });
});

$('#emailTest').submit(function () {
  $('#emailTestBtnspinner').removeClass('d-none');
  $('#emailTestBtnText').text('发送中...');
  $('#emailTestBtn').prop('disabled', true);

  request
    .post('/email/send', {
      email: $('#emailTestTo').val(),
      subject: 'Test',
      content: '测试邮件！',
    })
    .then((data) => {
      message.success(data.message);
    })
    .finally(() => {
      $('#emailTestBtnspinner').addClass('d-none');
      $('#emailTestBtnText').text('发送测试');
      $('#emailTestBtn').prop('disabled', false);
    });

  return false;
});

// client url
$(document).on('focus', '#windowsClientUrl', function () {
  $(this).data('oldValue', $(this).val());
});

$(document).on('blur', '#windowsClientUrl', function () {
  const oldValue = $(this).data('oldValue');
  const newValue = $(this).val();

  if (oldValue === newValue) return;

  request.post('/settings', { 'client.client_url.windows': newValue }).then((data) => {
    message.success(data.message);
  });
});

$(document).on('focus', '#macosClientUrl', function () {
  $(this).data('oldValue', $(this).val());
});

$(document).on('blur', '#macosClientUrl', function () {
  const oldValue = $(this).data('oldValue');
  const newValue = $(this).val();

  if (oldValue === newValue) return;

  request.post('/settings', { 'client.client_url.macos': newValue }).then((data) => {
    message.success(data.message);
  });
});

$(document).on('focus', '#linuxClientUrl', function () {
  $(this).data('oldValue', $(this).val());
});

$(document).on('blur', '#linuxClientUrl', function () {
  const oldValue = $(this).data('oldValue');
  const newValue = $(this).val();

  if (oldValue === newValue) return;

  request.post('/settings', { 'client.client_url.linux': newValue }).then((data) => {
    message.success(data.message);
  });
});

$(document).on('focus', '#androidClientUrl', function () {
  $(this).data('oldValue', $(this).val());
});

$(document).on('blur', '#androidClientUrl', function () {
  const oldValue = $(this).data('oldValue');
  const newValue = $(this).val();

  if (oldValue === newValue) return;

  request.post('/settings', { 'client.client_url.android': newValue }).then((data) => {
    message.success(data.message);
  });
});

$(document).on('focus', '#iosClientUrl', function () {
  $(this).data('oldValue', $(this).val());
});

$(document).on('blur', '#iosClientUrl', function () {
  const oldValue = $(this).data('oldValue');
  const newValue = $(this).val();

  if (oldValue === newValue) return;

  request.post('/settings', { 'client.client_url.ios': newValue }).then((data) => {
    message.success(data.message);
  });
});

// openvpn
$(document).on('focus', '#ovpnPort', function () {
  $(this).data('oldValue', $(this).val());
});

$(document).on('blur', '#ovpnPort', function () {
  const oldValue = $(this).data('oldValue');
  const newValue = $(this).val();

  if (oldValue === newValue) return;

  request.post('/settings', { 'openvpn.ovpn_port': newValue }).then((data) => {
    message.success(data.message);
  });
});

$(document).on('focus', '#ovpnProto', function () {
  $(this).data('oldValue', $(this).val());
});

$(document).on('blur', '#ovpnProto', function () {
  const oldValue = $(this).data('oldValue');
  const newValue = $(this).val();

  if (oldValue === newValue) return;

  request.post('/settings', { 'openvpn.ovpn_proto': newValue }).then((data) => {
    message.success(data.message);
  });
});

$(document).on('focus', '#ovpnMaxClients', function () {
  $(this).data('oldValue', $(this).val());
});

$(document).on('blur', '#ovpnMaxClients', function () {
  const oldValue = $(this).data('oldValue');
  const newValue = $(this).val();

  if (oldValue === newValue) return;

  request.post('/settings', { 'openvpn.ovpn_max_clients': newValue }).then((data) => {
    message.success(data.message);
  });
});

$(document).on('focus', '#ovpnSubnet', function () {
  $(this).data('oldValue', $(this).val());
});

$(document).on('blur', '#ovpnSubnet', function () {
  const oldValue = $(this).data('oldValue');
  const newValue = $(this).val();

  if (oldValue === newValue) return;

  request.post('/settings', { 'openvpn.ovpn_subnet': newValue }).then((data) => {
    message.success(data.message);
  });
});

$(document).on('change', '#ovpnGateway', function () {
  request.post('/settings', { 'openvpn.ovpn_gateway': $(this).prop('checked') }).then((data) => {
    message.success(data.message);
  });
});

$(document).on('focus', '#ovpnManagement', function () {
  $(this).data('oldValue', $(this).val());
});

$(document).on('blur', '#ovpnManagement', function () {
  const oldValue = $(this).data('oldValue');
  const newValue = $(this).val();

  if (oldValue === newValue) return;

  request.post('/settings', { 'openvpn.ovpn_management': newValue }).then((data) => {
    message.success(data.message);
  });
});

$(document).on('change', '#ovpnIpv6', function () {
  request.post('/settings', { 'openvpn.ovpn_ipv6': $(this).prop('checked') }).then((data) => {
    message.success(data.message);
  });
});

$(document).on('focus', '#ovpnSubnet6', function () {
  $(this).data('oldValue', $(this).val());
});

$(document).on('blur', '#ovpnSubnet6', function () {
  const oldValue = $(this).data('oldValue');
  const newValue = $(this).val();

  if (oldValue === newValue) return;

  request.post('/settings', { 'openvpn.ovpn_subnet6': newValue }).then((data) => {
    message.success(data.message);
  });
});

$(document).on('focus', '#ovpnPushDns1', function () {
  $(this).data('oldValue', $(this).val());
});

$(document).on('blur', '#ovpnPushDns1', function () {
  const oldValue = $(this).data('oldValue');
  const newValue = $(this).val();

  if (oldValue === newValue) return;

  request.post('/settings', { 'openvpn.ovpn_push_dns1': newValue }).then((data) => {
    message.success(data.message);
  });
});

$(document).on('focus', '#ovpnPushDns2', function () {
  $(this).data('oldValue', $(this).val());
});

$(document).on('blur', '#ovpnPushDns2', function () {
  const oldValue = $(this).data('oldValue');
  const newValue = $(this).val();

  if (oldValue === newValue) return;

  request.post('/settings', { 'openvpn.ovpn_push_dns2': newValue }).then((data) => {
    message.success(data.message);
  });
});
