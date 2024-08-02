import React from 'react';
import PropTypes from 'prop-types';

import Nav from 'react-bootstrap/Nav';

import HuntOverview from './hunt-overview.jsx';
import HuntRequest from './hunt-request.jsx';
import HuntClients from './hunt-clients.jsx';
import HuntNotebook from './hunt-notebook.jsx';
import Spinner from '../utils/spinner.jsx';
import T from '../i8n/i8n.jsx';
import { withRouter }  from "react-router-dom";

class HuntInspector extends React.Component {
    static propTypes = {
        hunt: PropTypes.object,
        fetch_hunts: PropTypes.func,

        // React router props.
        match: PropTypes.object,
        history: PropTypes.object,
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
        if (this.props.hunt && this.props.hunt.loading) {
            return <Spinner loading={true}/>;
        }

        if (!this.props.hunt || !this.props.hunt.hunt_id) {
            return <div className="no-content">{T("Please select a hunt above")}</div>;
        }

        // Default tab comes from the router
        let tab = this.props.match && this.props.match.params &&
            this.props.match.params.tab;
        tab = tab || this.state.tab;

        return (
            <div className="padded">
              <Nav variant="tab" defaultActiveKey={tab}
                   onSelect={this.setDefaultTab}>
                <Nav.Item>
                  <Nav.Link as="button" className="btn btn-default"
                            eventKey="overview">{T("Overview")}</Nav.Link>
                </Nav.Item>
                <Nav.Item>
                  <Nav.Link as="button" className="btn btn-default"
                            eventKey="requests">{T("Requests")}</Nav.Link>
                </Nav.Item>
                <Nav.Item>
                  <Nav.Link as="button" className="btn btn-default"
                            eventKey="clients">{T("Clients")}</Nav.Link>
                </Nav.Item>
                <Nav.Item>
                  <Nav.Link as="button" className="btn btn-default"
                            eventKey="notebook">{T("Notebook")}</Nav.Link>
                </Nav.Item>
              </Nav>

              { tab === "overview" &&
                <HuntOverview
                  fetch_hunts={this.props.fetch_hunts}
                  hunt={this.props.hunt}/> }
              { tab === "requests" &&
                <HuntRequest hunt={this.props.hunt}/> }
              { tab === "clients" &&
                <HuntClients hunt={this.props.hunt} />}
              { tab === "notebook" &&
                <HuntNotebook hunt={this.props.hunt} />}
            </div>
        );
    }
};

export default withRouter(HuntInspector);
