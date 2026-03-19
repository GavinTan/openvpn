tables.firewall = {
  scrollY: 'calc(100vh - 300px)',
  scrollCollapse: true,
  paging: false,
  autoWidth: false,
  responsive: false,
  columns: [
    {
      title: 'ID',
      data: 'id',
    },
    {
      title: '源地址',
      data: 'sip',
      render: (data, type, row) => {
        const html = [];
        if (data) html.push(...data.split(','));

        if (row.sg.length > 0) {
          html.push(...row.sg.sort((a, b) => new Date(b.updatedAt) - new Date(a.updatedAt)).map((g) => g.name));
        }

        return html.join('<br />');
      },
    },
    {
      title: '目的地址',
      data: 'dip',
      render: (data, type, row) => {
        const html = [];
        if (data) html.push(...data.split(','));

        if (row.dg.length > 0) {
          html.push(...row.dg.sort((a, b) => new Date(b.updatedAt) - new Date(a.updatedAt)).map((g) => g.name));
        }

        return html.join('<br />');
      },
    },
    {
      title: '策略',
      data: 'policy',
    },
    {
      title: '状态',
      data: 'status',
      render: (data, type, row) => {
        return data === true
          ? '<span class="badge text-bg-success">启用</span>'
          : '<span class="badge text-bg-danger">禁用</span>';
      },
    },
    { title: '备注', data: 'comment', className: 'dt-center w-max-200 text-truncate' },
    {
      title: '创建时间',
      data: 'createdAt',
      render: (data, type, row) => dayjs(data).format('YYYY-MM-DD HH:mm:ss'),
    },
    {
      title: '操作',
      data: null,
      orderable: false,
      searchable: false,
      render: (data, type, row) => {
        const html = `
          <div class="d-flex gap-2 justify-content-center align-items-center">
            <button class="btn btn-link text-decoration-none p-0" type="button" id="editFirewall">编辑</button>
            ${
              row.status === true
                ? '<button class="btn btn-link text-decoration-none p-0" type="button" id="disableFirewall">禁用</button>'
                : '<button class="btn btn-link text-decoration-none p-0" type="button" id="enableFirewall">启用</button>'
            }
            <button class="btn btn-link text-decoration-none p-0 btn-delete" data-bs-toggle="popover" data-delete-type="firewall" data-delete-name="${
              row.id
            }">删除</button>
          </div>
          `;
        return html;
      },
    },
  ],
  order: [[6, 'desc']],
  buttons: {
    dom: {
      button: { className: 'btn btn-sm' },
    },
    buttons: [
      {
        text: '添加',
        className: 'btn-primary',
        action: () => {
          $('#addFirewallModal select:not([name="policy"])').empty();
          $('#addFirewallModal form').trigger('reset');
          request.get('/ovpn/group').then((data) => {
            data.forEach((i) => {
              $('<option>', { value: i.id, text: i.name }).appendTo("#addFirewallModal select[name='sug']");
              // $('<option>', { value: i.id, text: i.name }).appendTo("#addFirewallModal select[name='dug']");
            });

            $("#addFirewallModal input[name='sip']").toggleClass('border border-danger', false);
            $("#addFirewallModal input[name='dip']").toggleClass('border border-danger', false);

            $('#addFirewallModal').modal('show');
          });
        },
      },
    ],
  },
  dom:
    "<'d-flex justify-content-between align-items-center'fB>" +
    "<'row'<'col-sm-12'tr>>" +
    "<'d-flex justify-content-between align-items-center'lip>",
  drawCallback: function (settings) {
    $('#vtable .btn-delete').popover('dispose');
    $('#vtable .btn-delete').popover({
      container: 'body',
      placement: 'top',
      html: true,
      sanitize: false,
      trigger: 'click',
      title: '提示',
      content: function (e) {
        const name = $(e).data('delete-name');
        return `
            <div>
              <p>确认删除 <strong>${name}</strong> 吗？</p>
              <div class="d-flex justify-content-center">
                <button class="btn btn-secondary btn-sm me-2 btn-popover-cancel">取消</button>
                <button class="btn btn-primary btn-sm btn-popover-confirm">确认</button>
              </div>
            </div>
          `;
      },
    });
  },
  ajax: function (data, callback, settings) {
    request.get('/ovpn/firewall').then((data) => callback({ data }));
  },
};

function isValidIPOrRange(val) {
  if (typeof val !== 'string') return false;
  const input = val.trim();

  const ipv4Part = '(25[0-5]|2[0-4]\\d|1\\d\\d|[1-9]\\d|\\d)';
  const ipv4Regex = new RegExp(`^(${ipv4Part}\\.){3}${ipv4Part}$`);
  const ipv6Regex =
    /^(([0-9a-fA-F]{1,4}:){7,7}[0-9a-fA-F]{1,4}|([0-9a-fA-F]{1,4}:){1,7}:|([0-9a-fA-F]{1,4}:){1,6}:[0-9a-fA-F]{1,4}|([0-9a-fA-F]{1,4}:){1,5}(:[0-9a-fA-F]{1,4}){1,2}|([0-9a-fA-F]{1,4}:){1,4}(:[0-9a-fA-F]{1,4}){1,3}|([0-9a-fA-F]{1,4}:){1,3}(:[0-9a-fA-F]{1,4}){1,4}|([0-9a-fA-F]{1,4}:){1,2}(:[0-9a-fA-F]{1,4}){1,5}|[0-9a-fA-F]{1,4}:((:[0-9a-fA-F]{1,4}){1,6})|:((:[0-9a-fA-F]{1,4}){1,7}|:)|fe80:(:[0-9a-fA-F]{0,4}){0,4}%[0-9a-zA-Z]{1,}|::(ffff(:0{1,4}){0,1}:){0,1}((25[0-5]|(2[0-4]|1{0,1}[0-9]){0,1}[0-9])\\.){3,3}(25[0-5]|(2[0-4]|1{0,1}[0-9]){0,1}[0-9])|([0-9a-fA-F]{1,4}:){1,4}:((25[0-5]|(2[0-4]|1{0,1}[0-9]){0,1}[0-9])\\.){3,3}(25[0-5]|(2[0-4]|1{0,1}[0-9]){0,1}[0-9]))$/;

  const ipv4ToBigInt = (ip) => {
    return ip.split('.').reduce((acc, octet) => (acc << 8n) + BigInt(octet), 0n);
  };

  const ipv6ToBigInt = (ip) => {
    let fullIp = ip;
    if (ip.includes('::')) {
      const parts = ip.split('::');
      const left = parts[0] ? parts[0].split(':') : [];
      const right = parts[1] ? parts[1].split(':') : [];
      const missing = new Array(8 - (left.length + right.length)).fill('0');
      fullIp = [...left, ...missing, ...right].join(':');
    }
    return fullIp.split(':').reduce((acc, hex) => (acc << 16n) + BigInt(parseInt(hex || '0', 16)), 0n);
  };

  if (input.includes('/')) {
    const [ip, maskStr] = input.split('/');
    const mask = parseInt(maskStr, 10);
    if (isNaN(mask)) return false;
    if (ipv4Regex.test(ip)) return mask >= 0 && mask <= 32;
    if (ipv6Regex.test(ip)) return mask >= 0 && mask <= 128;
    return false;
  }

  if (input.includes('-')) {
    const [start, end] = input.split('-').map((s) => s.trim());
    if (ipv4Regex.test(start) && ipv4Regex.test(end)) {
      return ipv4ToBigInt(start) <= ipv4ToBigInt(end);
    }
    if (ipv6Regex.test(start) && ipv6Regex.test(end)) {
      return ipv6ToBigInt(start) <= ipv6ToBigInt(end);
    }
    return false;
  }

  return ipv4Regex.test(input) || ipv6Regex.test(input);
}

$("#addFirewallModal input[name='sip']").on('input', function () {
  const val = $(this).val();
  $(this).toggleClass('border border-danger', val !== '' && !isValidIPOrRange(val));
});

$("#addFirewallModal input[name='dip']").on('input', function () {
  const val = $(this).val();
  $(this).toggleClass('border border-danger', val !== '' && !isValidIPOrRange(val));
});

$('#addFirewallModal button[name="saddBtn"]').click(function () {
  const sip = $("#addFirewallModal input[name='sip']").val();
  if (sip && isValidIPOrRange(sip)) {
    if ($("#addFirewallModal select[name='sg']").find(`option[value='${sip}']`).length === 0) {
      $('<option>', { value: sip, text: sip })
        .attr('data-source', 'input')
        .appendTo("#addFirewallModal select[name='sg']");
    }

    $("#addFirewallModal input[name='sip']").val('');
  }

  $("#addFirewallModal select[name='sug']")
    .find('option:selected')
    .each(function () {
      if ($("#addFirewallModal select[name='sg']").find(`option[value='${$(this).val()}']`).length === 0) {
        $(this).appendTo("#addFirewallModal select[name='sg']");
      }
    });

  $("#addFirewallModal select[name='sug']").val([]);
  $("#addFirewallModal select[name='sg']").val([]);
});

$('#addFirewallModal button[name="sdelBtn"]').click(function () {
  $("#addFirewallModal select[name='sg']")
    .find('option:selected')
    .each(function () {
      if ($(this).attr('data-source') === 'input') {
        $(this).remove();
      } else {
        $(this).appendTo("#addFirewallModal select[name='sug']");
      }
    });

  $("#addFirewallModal select[name='sug']").val([]);
  $("#addFirewallModal select[name='sg']").val([]);
});

$('#addFirewallModal button[name="daddBtn"]').click(function () {
  const dip = $("#addFirewallModal input[name='dip']").val();
  if (dip && isValidIPOrRange(dip)) {
    if ($("#addFirewallModal select[name='dg']").find(`option[value='${dip}']`).length === 0) {
      $('<option>', { value: dip, text: dip })
        .attr('data-source', 'input')
        .appendTo("#addFirewallModal select[name='dg']");
    }

    $("#addFirewallModal input[name='dip']").val('');
  }

  $("#addFirewallModal select[name='dug']")
    .find('option:selected')
    .each(function () {
      if ($("#addFirewallModal select[name='dg']").find(`option[value='${$(this).val()}']`).length === 0) {
        $(this).appendTo("#addFirewallModal select[name='dg']");
      }
    });

  $("#addFirewallModal select[name='dug']").val([]);
  $("#addFirewallModal select[name='dg']").val([]);
});

$('#addFirewallModal button[name="ddelBtn"]').click(function () {
  $("#addFirewallModal select[name='dg']")
    .find('option:selected')
    .each(function () {
      if ($(this).attr('data-source') === 'input') {
        $(this).remove();
      } else {
        $(this).appendTo("#addFirewallModal select[name='dug']");
      }
    });

  $("#addFirewallModal select[name='dug']").val([]);
  $("#addFirewallModal select[name='dg']").val([]);
});

$('#addFirewallModal form').submit(function (e) {
  e.preventDefault();

  const sip = [];
  const dip = [];
  const sg = [];
  const dg = [];
  const policy = $("#addFirewallModal select[name='policy']").val();
  const comment = $("#addFirewallModal textarea[name='comment']").val();

  const sgOption = $("#addFirewallModal select[name='sg']").find('option');
  const dgOption = $("#addFirewallModal select[name='dg']").find('option');

  if (sgOption.length === 0 || dgOption.length === 0) {
    message.error('源地址或目的地址为空');
    return;
  }

  sgOption.each(function () {
    if ($(this).attr('data-source') === 'input') {
      sip.push($(this).val());
    } else {
      sg.push($(this).val());
    }
  });

  dgOption.each(function () {
    if ($(this).attr('data-source') === 'input') {
      dip.push($(this).val());
    } else {
      dg.push($(this).val());
    }
  });

  request.post('/ovpn/firewall', { sip, dip, sg, dg, policy, comment }).then((data) => {
    message.success(data.message);
    vtable.ajax.reload(null, false);
    $('#addFirewallModal').modal('hide');
  });
});

// 编辑防火墙规则
$(document).on('click', '#editFirewall', function () {
  $('#editFirewallModal select:not([name="policy"])').empty();
  $('#editFirewallModal form').trigger('reset');

  const data = vtable.row($(this).parents('tr')).data();
  data.sip.split(',').forEach((i) => {
    if (i)
      $('<option>', { value: i, text: i })
        .attr('data-source', 'input')
        .appendTo("#editFirewallModal select[name='sg']");
  });
  data.dip.split(',').forEach((i) => {
    if (i)
      $('<option>', { value: i, text: i })
        .attr('data-source', 'input')
        .appendTo("#editFirewallModal select[name='dg']");
  });

  data.sg.forEach((i) => {
    $('<option>', { value: i.id, text: i.name }).appendTo("#editFirewallModal select[name='sg']");
  });
  data.dg.forEach((i) => {
    $('<option>', { value: i.id, text: i.name }).appendTo("#editFirewallModal select[name='dg']");
  });

  $('#editFirewallModal input[name="id"]').val(data.id);
  $("#editFirewallModal input[name='sip']").toggleClass('border border-danger', false);
  $("#editFirewallModal input[name='dip']").toggleClass('border border-danger', false);
  $("#editFirewallModal select[name='policy']").val(data.policy);
  $("#editFirewallModal textarea[name='comment']").val(data.comment);
  $("#editFirewallModal input[name='status']").prop('checked', data.status);

  request.get('/ovpn/group').then((g) => {
    g.forEach((i) => {
      if (!data.sg.some((sg) => sg.id === i.id)) {
        $('<option>', { value: i.id, text: i.name }).appendTo("#editFirewallModal select[name='sug']");
      }
      // if (!data.dg.some((dg) => dg.id === i.id)) {
      //   $('<option>', { value: i.id, text: i.name }).appendTo("#editFirewallModal select[name='dug']");
      // }
    });

    $('#editFirewallModal').modal('show');
  });
});

$("#editFirewallModal input[name='sip']").on('input', function () {
  const val = $(this).val();
  $(this).toggleClass('border border-danger', val !== '' && !isValidIPOrRange(val));
});

$("#editFirewallModal input[name='dip']").on('input', function () {
  const val = $(this).val();
  $(this).toggleClass('border border-danger', val !== '' && !isValidIPOrRange(val));
});

$('#editFirewallModal button[name="saddBtn"]').click(function () {
  const sip = $("#editFirewallModal input[name='sip']").val();
  if (sip && isValidIPOrRange(sip)) {
    if ($("#editFirewallModal select[name='sg']").find(`option[value='${sip}']`).length === 0) {
      $('<option>', { value: sip, text: sip })
        .attr('data-source', 'input')
        .appendTo("#editFirewallModal select[name='sg']");
    }

    $("#editFirewallModal input[name='sip']").val('');
  }

  $("#editFirewallModal select[name='sug']")
    .find('option:selected')
    .each(function () {
      if ($("#editFirewallModal select[name='sg']").find(`option[value='${$(this).val()}']`).length === 0) {
        $(this).appendTo("#editFirewallModal select[name='sg']");
      }
    });

  $("#editFirewallModal select[name='sug']").val([]);
  $("#editFirewallModal select[name='sg']").val([]);
});

$('#editFirewallModal button[name="sdelBtn"]').click(function () {
  $("#editFirewallModal select[name='sg']")
    .find('option:selected')
    .each(function () {
      if ($(this).attr('data-source') === 'input') {
        $(this).remove();
      } else {
        $(this).appendTo("#editFirewallModal select[name='sug']");
      }
    });

  $("#editFirewallModal select[name='sug']").val([]);
  $("#editFirewallModal select[name='sg']").val([]);
});

$('#editFirewallModal button[name="daddBtn"]').click(function () {
  const dip = $("#editFirewallModal input[name='dip']").val();
  if (dip && isValidIPOrRange(dip)) {
    if ($("#editFirewallModal select[name='dg']").find(`option[value='${dip}']`).length === 0) {
      $('<option>', { value: dip, text: dip })
        .attr('data-source', 'input')
        .appendTo("#editFirewallModal select[name='dg']");
    }

    $("#editFirewallModal input[name='dip']").val('');
  }

  $("#editFirewallModal select[name='dug']")
    .find('option:selected')
    .each(function () {
      if ($("#editFirewallModal select[name='dg']").find(`option[value='${$(this).val()}']`).length === 0) {
        $(this).appendTo("#editFirewallModal select[name='dg']");
      }
    });

  $("#editFirewallModal select[name='dug']").val([]);
  $("#editFirewallModal select[name='dg']").val([]);
});

$('#editFirewallModal button[name="ddelBtn"]').click(function () {
  $("#editFirewallModal select[name='dg']")
    .find('option:selected')
    .each(function () {
      if ($(this).attr('data-source') === 'input') {
        $(this).remove();
      } else {
        $(this).appendTo("#editFirewallModal select[name='dug']");
      }
    });

  $("#editFirewallModal select[name='dug']").val([]);
  $("#editFirewallModal select[name='dg']").val([]);
});

$('#editFirewallModal form').submit(function (e) {
  e.preventDefault();

  const sip = [];
  const dip = [];
  const sg = [];
  const dg = [];
  const policy = $("#editFirewallModal select[name='policy']").val();
  const comment = $("#editFirewallModal textarea[name='comment']").val();
  const id = $("#editFirewallModal input[name='id']").val();
  const status = $("#editFirewallModal input[name='status']").prop('checked');

  const sgOption = $("#editFirewallModal select[name='sg']").find('option');
  const dgOption = $("#editFirewallModal select[name='dg']").find('option');

  if (sgOption.length === 0 || dgOption.length === 0) {
    message.error('源地址或目的地址为空');
    return;
  }

  sgOption.each(function () {
    if ($(this).attr('data-source') === 'input') {
      sip.push($(this).val());
    } else {
      sg.push($(this).val());
    }
  });

  dgOption.each(function () {
    if ($(this).attr('data-source') === 'input') {
      dip.push($(this).val());
    } else {
      dg.push($(this).val());
    }
  });

  request.patch('/ovpn/firewall', { id, sip, dip, sg, dg, policy, comment, status }).then((data) => {
    message.success(data.message);
    vtable.ajax.reload(null, false);
    $('#editFirewallModal').modal('hide');
  });
});

// 禁用规则
$(document).on('click', '#disableFirewall', function () {
  const data = vtable.row($(this).parents('tr')).data();

  request
    .patch('/ovpn/firewall', {
      ...data,
      status: false,
      sg: data.sg.map((i) => i.id).join(','),
      dg: data.dg.map((i) => i.id).join(','),
    })
    .then((data) => {
      message.success(data.message);
      vtable.ajax.reload(null, false);
    });
});

// 启用规则
$(document).on('click', '#enableFirewall', function () {
  const data = vtable.row($(this).parents('tr')).data();

  request
    .patch('/ovpn/firewall', {
      ...data,
      status: true,
      sg: data.sg.map((i) => i.id).join(','),
      dg: data.dg.map((i) => i.id).join(','),
    })
    .then((data) => {
      message.success(data.message);
      vtable.ajax.reload(null, false);
    });
});
