package fhassets

// Report is the index.html written to the web root
const Report = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1, shrink-to-fit=no">
  <title>FIO {{.Description}} Health</title>
  <link rel="stylesheet" href="bootstrap.min.css">
  <link rel="stylesheet" href="https://unpkg.com/bootstrap-table@1.18.0/dist/bootstrap-table.min.css">
  <script src="https://cdn.jsdelivr.net/npm/echarts@4.9.0/dist/echarts.js"></script>
  <style type="text/css">
    html {
      scroll-behavior: smooth;
    }
  </style>
  <script>
  </script>
</head>
<body>
<div class="container-fluid">
  <div class="w-95 mx-auto" style="max-width: 1600px;">
    <h1>{{.Description}} Health</h1>
    <div><br /></div>
    <div class="text-info">Last run: {{.Timestamp}}</div>
    <div id="history-button">
      <br />
      <button type="button" class="btn btn-primary" data-toggle="modal" data-target="#historyModal">
        Previous Reports
      </button>
      <!-- Modal -->
      <div class="modal fade" id="historyModal" tabindex="-1" aria-labelledby="historyModalLabel" aria-hidden="true">
        <div class="modal-dialog">
          <div class="modal-content">
            <div class="modal-header">
              <h5 class="modal-title" id="historyModalLabel">Previous Reports</h5>
              <button type="button" class="close" data-dismiss="modal" aria-label="Close">
                <span aria-hidden="true">&times;</span>
              </button>
            </div>
            <div class="modal-body" id="history">
            </div>
            <div class="modal-footer">
              <button type="button" class="btn btn-secondary" data-dismiss="modal">Close</button>
            </div>
          </div>
        </div>
      </div>
    </div>

	<div>
      <br />
      <div id="chart" class="border-dark mx-auto rounded-lg" style="width: 1000px;height:300px; background-color: #303030;" hidden>
        Please wait, loading ....
      </div>
    </div>

    <div><br /></div>
      <h2>API</h2>
      <table onChange="removeButtons" class="table table-striped table-sm table-hover table-borderless" data-toggle="table" data-search="true" data-custom-sort="customSort">
        <thead class="thead-dark">
          <tr>
            <th scope="col">Host</th>
            <th scope="col">Version</th>
            <th scope="col">Healthy</th>
            <th scope="col">Errors</th>
            <th scope="col"></th>
            <th scope="col" data-sortable="true">Response (ms)</th>
            <th scope="col"></th>
            <th scope="col" data-sortable="true">Headblock Lag (ms)</th>
            <th scope="col">CORS</th>
            <th scope="col">Strong TLS</th>
            <th scope="col">TLS Info</th>
            <th class="text-center" scope="col">Security Warnings</th>
            <th scope="col">Test Origin</th>
          </tr>
        </thead>
        <tbody>
        {{range .Api}}
        <tr id="{{.Node}}">
          <th scope="row" class="align-middle">{{.Node}}</th>
          <th scope="row" {{if .WrongVersion}}class="align-middle text-warning"{{else}}class="align-middle"{{end}}>{{.NodeVer}}</th>
          <td class="align-middle">{{if .HadError}}<img src="tri.svg" alt="failed" width="28" height="28">{{else}}<img src="check.svg" alt="ok" width="28" height="28">{{end}}</td>
          <td class="text-info" style="max-width: 250px;"><div class="d-inline-block overflow-hidden" style="max-width: 245px;max-height: 40px;" >
          <span data-toggle="tooltip" delay="0" trigger="hover focus" placement="right" title="{{.Error}}">
              {{.Error}}
          </span>
          </div></td>
          <td class="align-middle">
            <div>
              <button type="button" class="chart-button btn btn-outline-dark" onClick="graphLatency('{{.Node}}')">
                <img src="chart.svg" alt="view latency chart" class="align-middle" width="20" height="20">
              </button>
             </div>
          </td>
          <td {{if gt .RequestLatency 2000}}class="align-middle text-warning"{{else}}class="align-middle"{{end}}>
            <div>
			  {{.RequestLatency}} 
             </div>
           </td>
          <td class="align-middle">
            <div>
              <button type="button" class="chart-button btn btn-outline-dark" onClick="graphLatency('{{.Node}}', 'lag')">
                <img src="chart.svg" alt="view latency chart" class="align-middle" width="20" height="20">
              </button>
             </div>
          </td>
          <td {{if gt .HeadBlockLatency 30000 }} class="text-warning align-middle"{{else}} class="align-middle"{{end}}>
              {{.HeadBlockLatency}}
             </div>
          </td>
          <td class="align-middle">{{if .PermissiveCors}}<img src="check.svg" alt="ok" width="28" height="28">{{else}}<img src="tri.svg" alt="failed" width="28" height="28">{{end}}</td>
          <td class="align-middle">{{ if not .TlsVerOk}}<img src="slash.svg" alt="failed" width="28" height="28">{{else if not .TlsCipherOk}}<img src="slash.svg" alt="failed" width="28" height="28">{{else}}<img src="check.svg" alt="ok" width="28" height="28">{{end}}</td>
          <td class="align-middle" style="max-width: 250px;"><div class="d-inline-block overflow-hidden" style="max-width: 245px;max-height: 40px;">
          <span data-toggle="tooltip" delay="0" trigger="hover focus" placement="right" title="{{.TlsNote}}">
              {{.TlsNote}}
          </span>
          </div></td>
          <td class="text-center align-middle">{{if .ProducerExposed}}<img src="exc.svg" alt="failed" width="28" height="28">{{else if .NetExposed}}<img src="exc.svg" alt="failed" width="28" height="28">{{end}}</td>
          <td>{{.FromGeo}}</td>
        </tr>
        {{ end }}
        </tbody>
      </table>
    <br />
    <h2>P2P</h2>
      <table class="table table-striped table-sm table-hover table-borderless" data-toggle="table" data-search="true" data-custom-sort="customSort">
        <thead class="thead-dark">
        <tr>
          <th scope="col">Host</th>
          <th scope="col">Listening</th>
          <th scope="col">Healthy</th>
          <th scope="col">Errors</th>
          <th scope="col">Headblock Lag (ms)</th>
          <th scope="col">Test Origin</th>
        </tr>
        </thead>
        <tbody>
        {{range .P2p}}
        <tr id="{{.Peer}}">
          <th scope="row">{{.Peer}}</th>
          <td>{{if .Reachable}}<img src="check.svg" alt="ok" width="28" height="28">{{else}}<img src="tri.svg" alt="failed" width="28" height="28">{{end}}</td>
          <td>{{if .Healthy}}<img src="check.svg" alt="ok" width="28" height="28">{{else}}<img src="tri.svg" alt="failed" width="28" height="28">{{end}}</td>
          <td class="text-info" style="max-width: 250px;"><div class="d-inline-block overflow-hidden" style="max-width: 245px;max-height: 40px;">
          <span href="#" data-toggle="tooltip" delay="0" title="{{.ErrMsg}}">
              {{.ErrMsg}}
          </span>
          </div></td>
          <td>{{if .Healthy}}{{.HeadBlockLatency}}{{end}}</td>
          <td>{{.FromGeo}}</td>
        </tr>
        {{end}}
        </tbody>
      </table>
  </div>
  </div>
  <script>
    let showingButtons = false
    const removeButtons = function() {
      if (showingButtons) {
        return
      }
      //document.getElementById("history-button").hidden = true;
      document.getElementById("history-button").remove();
      for (let b of document.getElementsByClassName("chart-button")){
        //b.hidden = true;
        b.remove();
      }
    }
    const previous = async function() {
      let response = await fetch("history/index.json");
      if (response.ok) {
        showingButtons = true;
        let json = await response.json();
        if (json.length === 0) {
          return
        }
        json.sort(function (a, b) {
          let result = 0
			if (a.file > b.file) {
			  result = -1
            } else if (a.file < b.file) {
			  result = 1
            }
			return result
        });
        let innerContent = "<ul>\n";
        for (const l of json) {
          innerContent += '<li><a href="history/' + l.file + '">' + l.date + " - " + l.from + "</a></li>\n"
        }
        innerContent += "</ul>\n";
        document.getElementById("history").insertAdjacentHTML("afterend", innerContent);
      } else {
        removeButtons();
      }
    };
    window.onload = function() {
      previous()
    };
    function extractContent(s) {
      let span = document.createElement('span');
      span.innerHTML = s;
      return span.textContent || span.innerText;
    }
    function customSort(sortName, sortOrder, data) {
      let order = sortOrder === 'desc' ? -1 : 1
      data.sort(function (a, b) {
        let aa = parseInt(extractContent(a[sortName]).replace(/[^\d]/g, ''));
        let bb = parseInt(extractContent(b[sortName]).replace(/[^\d]/g, ''));
        if (aa < bb) {
          return order * -1
        }
        if (aa > bb) {
          return order
        }
        return 0
      }
    );
    }
  </script>
  <script src="https://code.jquery.com/jquery-3.5.1.slim.min.js" integrity="sha384-DfXdz2htPH0lsSSs5nCTpuj/zy4C+OGpamoFVy38MVBnE+IbbVYUew+OrCXaRkfj" crossorigin="anonymous"></script>
  <script src="https://cdn.jsdelivr.net/npm/popper.js@1.16.1/dist/umd/popper.min.js" integrity="sha384-9/reFTGAW83EW2RDu2S0VKaIzap3H66lZH81PoYlFhbGU+6BZp6G7niu735Sk7lN" crossorigin="anonymous"></script>
  <script src="https://stackpath.bootstrapcdn.com/bootstrap/4.5.2/js/bootstrap.min.js" integrity="sha384-B4gt1jrGC7Jh4AgTPSdUtOBvfO8shuf57BaghqFfPlYxofvL8/KUEfYiJOMMV+rV" crossorigin="anonymous"></script>
  <script src="https://unpkg.com/bootstrap-table@1.18.0/dist/bootstrap-table.min.js"></script>
  <script src="chartv2.js"></script>

  <script>
  
  </script>
</body>
</html>`
