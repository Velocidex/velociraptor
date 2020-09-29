import React from 'react';
import PropTypes from 'prop-types';

import Tabs from 'react-bootstrap/Tabs';
import Tab from 'react-bootstrap/Tab';

import FlowOverview from './flow-overview.js';


export default class FlowInspector extends React.Component {
    static propTypes = {
        flow: PropTypes.object,
    };

    render() {
        return (
            <div className="padded">
              <Tabs defaultActiveKey="overview">
                <Tab eventKey="overview" title="Artifact Collection">
                  <FlowOverview flow={this.props.flow}/>
                </Tab>
                <Tab eventKey="uploads" title="Uploaded Files">
                </Tab>
                <Tab eventKey="requests" title="Requests">
                </Tab>
                <Tab eventKey="results" title="Results">
                </Tab>
                <Tab eventKey="logs" title="Log">
                </Tab>
              </Tabs>
              { JSON.stringify(this.props.flow)}
            </div>
        );
    }
};
