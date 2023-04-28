import React from 'react';
import PropTypes from 'prop-types';

import T from '../i8n/i8n.jsx';
import FlowNotebook from './flow-notebook.jsx';
import { withRouter }  from "react-router-dom";

import Navbar from 'react-bootstrap/Navbar';
import ButtonGroup from 'react-bootstrap/ButtonGroup';
import Button from 'react-bootstrap/Button';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';


class FullScreenFlowNotebook extends React.Component {
    static propTypes = {
        // React router props.
        match: PropTypes.object,
        history: PropTypes.object,
    };

    state = {};

    componentDidMount = () => {
        let flow_id = this.props.match && this.props.match.params &&
            this.props.match.params.flow_id;
        let client_id = this.props.match && this.props.match.params &&
            this.props.match.params.client_id;

        if (flow_id && client_id) {
            this.setState({
                flow_id: flow_id,
                client_id: client_id,
            });
        }
    }

    setSelectedNotebook = () => {
        this.props.history.push(
            "/collected/" + this.state.client_id + "/" +
                this.state.flow_id + "/notebook");
    }

    render() {
        return <>
                 <Navbar className="toolbar">
                   <ButtonGroup className="float-right floating-button">
                     <Button title={T("Exit Fullscreen")}
                             onClick={this.setSelectedNotebook}
                             variant="default">
                       <FontAwesomeIcon icon="compress"/>
                     </Button>
                   </ButtonGroup>
                 </Navbar>
                 <div className="fill-parent no-margins selectable">
                   <FlowNotebook
                     flow={{
                         client_id: this.state.client_id,
                         session_id: this.state.flow_id,
                     }}
                   />
                 </div>
               </>;
    }
};

export default withRouter(FullScreenFlowNotebook);
