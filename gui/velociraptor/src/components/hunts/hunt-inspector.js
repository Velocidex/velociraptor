import React from 'react';
import PropTypes from 'prop-types';

import Tabs from 'react-bootstrap/Tabs';
import Tab from 'react-bootstrap/Tab';

import HuntOverview from './hunt-overview.js';
import HuntRequest from './hunt-request.js';
import HuntClients from './hunt-clients.js';

import { withRouter }  from "react-router-dom";

class HuntInspector extends React.Component {
    static propTypes = {
        hunt: PropTypes.object,
    };

    state = {
        tab: "overview",
    }

    setDefaultTab = (tab) => {
        this.setState({tab: tab});
        this.props.history.push(
            "/hunts/" + this.props.hunt.hunt_id + "/" + tab);
    }

    render() {
        if (!this.props.hunt || !this.props.hunt.hunt_id) {
            return <div className="no-content">Please select a hunt above</div>;
        }

        // Default tab comes from the router
        let tab = this.props.match && this.props.match.params &&
            this.props.match.params.tab;
        tab = tab || this.state.tab;

        return (
            <div className="padded">
              <Tabs defaultActiveKey={tab} onSelect={this.setDefaultTab}>
                <Tab eventKey="overview" title="Overview">
                  { tab === "overview" &&
                    <HuntOverview hunt={this.props.hunt}/> }
                </Tab>
                <Tab eventKey="requests" title="Requests">
                  { tab === "requests" &&
                    <HuntRequest hunt={this.props.hunt}/> }
                </Tab>
                <Tab eventKey="clients" title="Clients">
                  { tab === "clients" &&
                    <HuntClients hunt={this.props.hunt} />}
                </Tab>
              </Tabs>
            </div>
        );
    }
};

export default withRouter(HuntInspector);
