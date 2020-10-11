import "./user-dashboard.css";

import React from 'react';
import PropTypes from 'prop-types';

import _ from 'lodash';
import Navbar from 'react-bootstrap/Navbar';
import ButtonGroup from 'react-bootstrap/ButtonGroup';
import Dropdown from 'react-bootstrap/Dropdown';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import VeloReportViewer from "../artifacts/reporting.js";

const ranges = [
    {desc: "Last Day", sec: 60*60*24, sample: 4},
    {desc: "Last 2 days", sec: 60*60*24*2, sample: 8},
    {desc: "Last Week", sec: 60*60*24*7, sample: 40},
  ];


export default class UserDashboard extends React.Component {
    static propTypes = {

    };

    constructor(props) {
        super(props);

        let now = parseInt((new Date()).getTime() / 1000);
        this.state = {
            start_time: now - ranges[0].sec,
            sample: 4
        };
    }

    setRange = (desc) => {
        // Current time in seconds.
        let now = parseInt((new Date()).getTime() / 1000);
        this.setState({start_time: now - desc.sec, sample: desc.sample});
    }

    render() {
        return (
            <>
              <Navbar className="toolbar">
                <ButtonGroup>
                  <Dropdown>
                    <Dropdown.Toggle variant="default">
                      <FontAwesomeIcon icon="book" />
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
                           sample: this.state.sample}}
                />
              </div>
            </>
        );
    }
};
