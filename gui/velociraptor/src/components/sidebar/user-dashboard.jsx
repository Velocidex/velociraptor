import "./user-dashboard.css";

import React from 'react';

import _ from 'lodash';
import Navbar from 'react-bootstrap/Navbar';
import Button from 'react-bootstrap/Button';
import ButtonGroup from 'react-bootstrap/ButtonGroup';
import Dropdown from 'react-bootstrap/Dropdown';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import VeloReportViewer from "../artifacts/reporting.jsx";
import T from '../i8n/i8n.jsx';

import { withRouter }  from "react-router-dom";

const ranges = [
    {desc: "Last Hour", sec: 60*60, sample: 1, rows: 400},
    {desc: "Last Day", sec: 60*60*24, sample: 6, rows: 2000},
    {desc: "Last 2 days", sec: 60*60*24*2, sample: 10, rows: 2000},
    {desc: "Last Week", sec: 60*60*24*7, sample: 40, rows: 2000},
  ];


class UserDashboard extends React.Component {
    static propTypes = {

    };

    constructor(props) {
        super(props);

        let now = parseInt((new Date()).getTime() / 1000);
        this.state = {
            start_time: now - ranges[0].sec,
            sample: ranges[0].sample,
            desc: ranges[0].desc,
            rows: ranges[0].rows,
            version: 0,
        };
    }

    shouldComponentUpdate = (nextProps, nextState) => {
        // Do not keep updating the dashboard - it is quite expensive
        // and should be done sparingly unless the version is actually
        // changed. The version is changed by pressing the "Refresh"
        // Button or selecting a new range.
        if (this.state.version !== nextState.version) {
            return true;
        }
        return false;
    };

    setRange = (desc) => {
        // Current time in seconds.
        let now = parseInt((new Date()).getTime() / 1000);
        this.setState({start_time: now - desc.sec,
                       desc: desc.desc,
                       rows: desc.rows,
                       version: this.state.version + 1,
                       sample: desc.sample});
    }

    render() {
        return (
            <>
              <Navbar className="toolbar">
                <ButtonGroup>
                  <Button variant="default"
                          data-position="right"
                          className="btn-tooltip"
                          data-tooltip={T("Redraw dashboard")}
                          onClick={() => this.setState({
                      version: this.state.version + 1,
                  })} >
                    <FontAwesomeIcon icon="sync"/>
                  </Button>

                  <Button variant="default"
                          data-position="right"
                          className="btn-tooltip"
                          data-tooltip={T("Edit the dashboard")}
                          onClick={() => {
                      this.props.history.push("/artifacts/Server.Monitor.Health");
                  }} >
                    <FontAwesomeIcon icon="pencil-alt"/>
                  </Button>
                </ButtonGroup>
                <ButtonGroup className="float-right">
                  <Dropdown>
                    <Dropdown.Toggle variant="default">
                      <FontAwesomeIcon icon="book" />
                      <span className="button-label">{this.state.desc}</span>
                    </Dropdown.Toggle>
                    <Dropdown.Menu>
                      { _.map(ranges, (x, idx) => {
                          return <Dropdown.Item key={idx}
                                                onClick={() => this.setRange(x)} >
                                   { x.desc }
                                 </Dropdown.Item>;
                      })}
                    </Dropdown.Menu>
                  </Dropdown>
                </ButtonGroup>
              </Navbar>
              <div className="dashboard">
                <VeloReportViewer
                  artifact="Custom.Server.Monitor.Health"
                  type="SERVER_EVENT"
                  params={{start_time: this.state.start_time,
                           version: this.state.version,
                           sample: this.state.sample}}
                />
              </div>
            </>
        );
    }
};

export default withRouter(UserDashboard);
