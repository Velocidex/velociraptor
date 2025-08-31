import "./user-dashboard.css";
import PropTypes from 'prop-types';

import React from 'react';

import _ from 'lodash';
import Navbar from 'react-bootstrap/Navbar';
import Button from 'react-bootstrap/Button';
import ButtonGroup from 'react-bootstrap/ButtonGroup';
import Dropdown from 'react-bootstrap/Dropdown';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import VeloReportViewer from "../artifacts/reporting.jsx";
import T from '../i8n/i8n.jsx';
import ToolTip from '../widgets/tooltip.jsx';

import { withRouter }  from "react-router-dom";

const ranges = [
    // Samples are taken every 10 sec by default (6 samples per
    // minute). Therefore the rows limit should be a fallback to
    // prevent really huge data sets.

    // Expected 6 * 60 = 360
    {desc: T("Last Hour"), sec: 60*60, sample: 1, rows: 400},

    // Expected 6 * 60 * 6 / 2 = 1080
    {desc: T("Last 6 Hours"), sec: 60*60*6, sample: 2, rows: 2000},

    // Expected 6 * 60 * 24 / 6 = 1440
    {desc: T("Last Day"), sec: 60*60*24, sample: 6, rows: 2000},

    // Expected 6 * 60 * 24 * 2 / 10 = 1728
    {desc: T("Last 2 days"), sec: 60*60*24*2, sample: 10, rows: 2000},

    // Expected 6 * 60 * 24 * 7 / 40 = 1512
    {desc: T("Last Week"), sec: 60*60*24*7, sample: 40, rows: 2000},
  ];


class UserDashboard extends React.Component {
    static propTypes = {
        // React router props.
        history: PropTypes.object,
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
                  <ToolTip tooltip={T("Redraw dashboard")}>
                    <Button variant="default"
                            onClick={() => this.setState({
                                version: this.state.version + 1,
                            })} >
                      <FontAwesomeIcon icon="sync"/>
                    </Button>
                  </ToolTip>
                  <ToolTip tooltip={T("Edit the dashboard")}>
                    <Button variant="default"
                            onClick={() => {
                                this.props.history.push(
                                    "/artifacts/Server.Monitor.Health/edit");
                            }} >
                      <FontAwesomeIcon icon="pencil-alt"/>
                    </Button>
                  </ToolTip>
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
                           parameters: [{name: "Sample",
                                         "default": this.state.sample.toString()}]}}
                />
              </div>
            </>
        );
    }
};

export default withRouter(UserDashboard);
