import React from 'react';
import HuntNotebook from './hunt-notebook.js';
import { withRouter }  from "react-router-dom";

import Navbar from 'react-bootstrap/Navbar';
import ButtonGroup from 'react-bootstrap/ButtonGroup';
import Button from 'react-bootstrap/Button';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import T from '../i8n/i8n.js';

class FullScreenHuntNotebook extends React.Component {
    static propTypes = {};

    state = {};

    componentDidMount = () => {
        let hunt_id = this.props.match && this.props.match.params &&
            this.props.match.params.hunt_id;
        if (hunt_id) {
            this.setState({hunt_id: hunt_id});
        }
    }

    setSelectedNotebook = () => {
        this.props.history.push("/hunts/" + this.state.hunt_id + "/notebook");
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
                   <HuntNotebook
                     hunt={{hunt_id: this.state.hunt_id}}
                   />
                 </div>
               </>;
    }
};

export default withRouter(FullScreenHuntNotebook);
