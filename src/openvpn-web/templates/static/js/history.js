tables.history = {
  columns: [
    { title: '用户名', data: 'username' },
    { title: '客户端', data: 'common_name' },
    { title: 'VPN IP', data: 'vip' },
    { title: '用户 IP', data: 'rip' },
    { title: '下载流量', data: 'bytes_received' },
    { title: '上传流量', data: 'bytes_sent' },
    { title: '上线时间', data: 'time_unix' },
    { title: '在线时长', data: 'time_duration' },
  ],
  order: [[6, 'desc']],
  processing: true,
  serverSide: true,
  search: {
    return: true,
  },
  dom:
    "<'row align-items-center'<'col d-flex'f><'col d-flex justify-content-center toolbar'><'col d-flex justify-content-end'B>>" +
    "<'row'<'col-sm-12'tr>>" +
    "<'d-flex justify-content-between align-items-center'lip>",
  fnInitComplete: function (oSettings, data) {
    $('#vtable_wrapper div.toolbar').html(
      `<div id="datepicker">
          <div class="input-group input-group-sm">
            <input type="text" class="form-control text-center" name="start" />
            <span class="input-group-text">to</span>
            <input type="text" class="form-control text-center" name="end" />
          </div>
        </div>
        `
    );

    const elem = document.getElementById('datepicker');
    const rangepicker = new DateRangePicker(elem, {
      buttonClass: 'btn',
      container: elem,
      format: 'yyyy-mm-dd',
      autohide: true,
      language: 'zh-CN',
    });

    rangepicker.setDates(lastMonth, now);

    elem.addEventListener('changeDate', (e) => {
      qt = rangepicker.getDates().map((d, i) => {
        if (d instanceof Date) {
          if (i == 1) {
            d.setHours(23, 59, 59, 0);
          }

          return Date.parse(d) / 1000;
        }

        return qt[i];
      });

      vtable.ajax.reload();
    });
  },
  buttons: {
    dom: {
      button: { className: 'btn btn-sm' },
    },
    buttons: [
      {
        text: '<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" fill="currentColor" class="bi bi-download" viewBox="0 0 16 16"> <path d="M.5 9.9a.5.5 0 0 1 .5.5v2.5a1 1 0 0 0 1 1h12a1 1 0 0 0 1-1v-2.5a.5.5 0 0 1 1 0v2.5a2 2 0 0 1-2 2H2a2 2 0 0 1-2-2v-2.5a.5.5 0 0 1 .5-.5"/> <path d="M7.646 11.854a.5.5 0 0 0 .708 0l3-3a.5.5 0 0 0-.708-.708L8.5 10.293V1.5a.5.5 0 0 0-1 0v8.793L5.354 8.146a.5.5 0 1 0-.708.708z"/> </svg> 导出',
        className: 'btn-primary',
        action: (e, dt, node, config, cb) =>
          (window.location.href = `/ovpn/history/export?${new URLSearchParams({ qt: qt.join() }).toString()}`),
      },
    ],
  },
  ajax: function (data, callback, settings) {
    const orderColumn = data.columns[data.order[0].column].data;
    const order = data.order[0].dir;
    const params = {
      draw: data.draw,
      offset: data.start,
      limit: data.length,
      orderColumn: orderColumn,
      order: order,
      search: data.search.value,
      qt: qt.join(),
    };

    request.get(`/ovpn/history?${new URLSearchParams(params).toString()}`).then((data) => callback(data));
  },
};
